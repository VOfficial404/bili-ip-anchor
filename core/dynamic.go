package core

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"
)

//go:embed assets/dynamic.jpg
var defaultDynamicImage []byte

const (
	maxImages    = 9
	maxImageSize = 10 << 20
)

// 动态图片放在数据目录的 dynamic/ 里，文件名由程序生成
var imageNameRe = regexp.MustCompile(`^[0-9]{1,20}-[0-9a-f]{8}\.(jpg|png|gif|webp)$`)

type imageInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

func (e *Engine) imagesDir() string { return filepath.Join(e.cfg.DataDir, "dynamic") }

// listImages 按添加顺序列出动态图片
func (e *Engine) listImages() ([]imageInfo, error) {
	entries, err := os.ReadDir(e.imagesDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []imageInfo
	for _, ent := range entries {
		if ent.IsDir() || !imageNameRe.MatchString(ent.Name()) {
			continue
		}
		info, err := ent.Info()
		if err != nil {
			continue
		}
		out = append(out, imageInfo{Name: ent.Name(), Size: info.Size()})
	}
	sort.Slice(out, func(i, j int) bool { return imageOrder(out[i].Name) < imageOrder(out[j].Name) })
	return out, nil
}

func imageOrder(name string) int64 {
	n, _ := strconv.ParseInt(name[:bytes.IndexByte([]byte(name), '-')], 10, 64)
	return n
}

// imageType 按文件头判断图片格式，返回扩展名
func imageType(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}):
		return "jpg"
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return "png"
	case bytes.HasPrefix(data, []byte("GIF87a")), bytes.HasPrefix(data, []byte("GIF89a")):
		return "gif"
	case len(data) > 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "webp"
	}
	return ""
}

var errTooManyImages = fmt.Errorf("动态图片最多 %d 张", maxImages)

// addImage 保存一张动态图片，返回文件名
func (e *Engine) addImage(data []byte) (string, error) {
	if len(data) > maxImageSize {
		return "", fmt.Errorf("图片不能超过 %d MB", maxImageSize>>20)
	}
	ext := imageType(data)
	if ext == "" {
		return "", errors.New("只支持 JPG、PNG、GIF、WebP 图片")
	}
	list, err := e.listImages()
	if err != nil {
		return "", err
	}
	if len(list) >= maxImages {
		return "", errTooManyImages
	}
	if err := os.MkdirAll(e.imagesDir(), 0700); err != nil {
		return "", err
	}
	// 文件名前缀决定顺序；Windows 的时钟精度不够，保证比已有的都大
	order := time.Now().UnixNano()
	if n := len(list); n > 0 && imageOrder(list[n-1].Name) >= order {
		order = imageOrder(list[n-1].Name) + 1
	}
	name := fmt.Sprintf("%d-%s.%s", order, randomHex(4), ext)
	return name, os.WriteFile(filepath.Join(e.imagesDir(), name), data, 0600)
}

func (e *Engine) readImage(name string) ([]byte, error) {
	if !imageNameRe.MatchString(name) {
		return nil, os.ErrNotExist
	}
	return os.ReadFile(filepath.Join(e.imagesDir(), name))
}

func (e *Engine) removeImage(name string) error {
	if !imageNameRe.MatchString(name) {
		return os.ErrNotExist
	}
	return os.Remove(filepath.Join(e.imagesDir(), name))
}

// migrateLegacyDynamicImage 把 v0.5 及更早版本数据目录里的 dynamic.jpg / dynamic.png 搬进 dynamic/
func (e *Engine) migrateLegacyDynamicImage() {
	for _, n := range []string{"dynamic.jpg", "dynamic.png"} {
		old := filepath.Join(e.cfg.DataDir, n)
		data, err := os.ReadFile(old)
		if err != nil {
			continue
		}
		name, err := e.addImage(data)
		if err != nil {
			e.Logf("旧的 %s 没有迁移：%v", old, err)
			continue
		}
		os.Remove(old)
		e.Logf("已把 %s 移到动态图片 %s", n, name)
	}
}

// dynamicImages 返回这轮动态要发的图片；没有文字也没有图片时用内置图片
func (e *Engine) dynamicImages(text string) ([][]byte, error) {
	list, err := e.listImages()
	if err != nil {
		return nil, err
	}
	var out [][]byte
	for _, img := range list {
		data, err := e.readImage(img.Name)
		if err != nil {
			return nil, err
		}
		out = append(out, data)
	}
	if len(out) == 0 && text == "" {
		out = [][]byte{defaultDynamicImage}
	}
	return out, nil
}

type uploadedPic struct {
	URL    string  `json:"image_url"`
	Width  int     `json:"image_width"`
	Height int     `json:"image_height"`
	Size   float64 `json:"img_size"` // KB
}

func (c *biliClient) uploadImage(csrf string, data []byte) (uploadedPic, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file_up", "image."+imageType(data))
	if err != nil {
		return uploadedPic{}, err
	}
	fw.Write(data)
	mw.WriteField("biz", "new_dyn")
	mw.WriteField("category", "daily")
	mw.WriteField("csrf", csrf)
	if err := mw.Close(); err != nil {
		return uploadedPic{}, err
	}
	req, err := http.NewRequest("POST", "https://api.bilibili.com/x/dynamic/feed/draw/upload_bfs", &buf)
	if err != nil {
		return uploadedPic{}, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	var r struct {
		apiResp
		Data uploadedPic `json:"data"`
	}
	if _, err := c.send(req, &r); err != nil {
		return uploadedPic{}, err
	}
	if r.Code != 0 || r.Data.URL == "" {
		return uploadedPic{}, fmt.Errorf("上传图片失败：%d %s", r.Code, r.Message)
	}
	return r.Data, nil
}

// dynamicRequest 组装发动态的请求：有图片时 scene 为 2（带图动态），只有文字时为 1
func dynamicRequest(text string, pics []uploadedPic) map[string]any {
	contents := []map[string]any{}
	if text != "" {
		contents = append(contents, map[string]any{"raw_text": text, "type": 1, "biz_id": ""})
	}
	req := map[string]any{
		"content": map[string]any{"contents": contents},
		"scene":   1,
		"meta":    map[string]any{"app_meta": map[string]any{"from": "create.dynamic.web", "mobi_app": "web"}},
	}
	if len(pics) > 0 {
		list := make([]map[string]any, 0, len(pics))
		for _, p := range pics {
			list = append(list, map[string]any{"img_src": p.URL, "img_width": p.Width, "img_height": p.Height, "img_size": p.Size})
		}
		req["pics"] = list
		req["scene"] = 2
	}
	return map[string]any{"dyn_req": req}
}

// removeDelay 发动态后等多久再删；测试里会改成 0
var removeDelay = func() time.Duration { return time.Duration(5+rand.Intn(16)) * time.Second }

func (e *Engine) postAndRemoveDynamic(c *biliClient, csrf, text string, logf logFunc) error {
	images, err := e.dynamicImages(text)
	if err != nil {
		return err
	}
	var pics []uploadedPic
	for _, data := range images {
		pic, err := c.uploadImage(csrf, data)
		if err != nil {
			return err
		}
		pics = append(pics, pic)
	}
	if len(pics) > 0 {
		logf("上传 %d 张图片", len(pics))
	}

	var cr struct {
		apiResp
		Data struct {
			DynID string `json:"dyn_id_str"`
		} `json:"data"`
	}
	if err := c.do("POST", "https://api.bilibili.com/x/dynamic/feed/create/dyn?csrf="+url.QueryEscape(csrf), nil, dynamicRequest(text, pics), &cr); err != nil {
		return err
	}
	logf("发动态：%d %s", cr.Code, cr.Message)
	if cr.Data.DynID == "" {
		return fmt.Errorf("发动态失败：%d %s", cr.Code, cr.Message)
	}

	time.Sleep(removeDelay())
	var rr apiResp
	if err := c.do("POST", "https://api.bilibili.com/x/dynamic/feed/operate/remove?csrf="+url.QueryEscape(csrf), nil,
		map[string]any{"dyn_id_str": cr.Data.DynID}, &rr); err != nil {
		return err
	}
	logf("删动态：%d %s", rr.Code, rr.Message)
	if rr.Code != 0 {
		return fmt.Errorf("删动态失败，请手动删除动态 %s：%d %s", cr.Data.DynID, rr.Code, rr.Message)
	}
	return nil
}

/* 心跳视频的信息，给设置页显示 */

type VideoInfo struct {
	BVID     string `json:"bvid"`
	Title    string `json:"title"`
	Cover    string `json:"cover"`
	Owner    string `json:"owner"`
	Duration int    `json:"duration"` // 秒
	Views    int64  `json:"views"`
}

type videoCache struct {
	info VideoInfo
	err  error
	at   time.Time
}

var errVideoNotFound = errors.New("找不到这个视频，可能不存在或不公开")

// videoInfo 查询视频信息，结果缓存 6 小时（失败缓存 5 分钟）
func (e *Engine) videoInfo(bvid string) (VideoInfo, error) {
	if !bvidRe.MatchString(bvid) {
		return VideoInfo{}, fmt.Errorf("BV 号格式不对：%q", bvid)
	}
	e.mu.Lock()
	cached, ok := e.videos[bvid]
	e.mu.Unlock()
	if ok && (cached.err == nil && time.Since(cached.at) < 6*time.Hour || time.Since(cached.at) < 5*time.Minute) {
		return cached.info, cached.err
	}
	var r struct {
		apiResp
		Data struct {
			BVID     string `json:"bvid"`
			Title    string `json:"title"`
			Pic      string `json:"pic"`
			Duration int    `json:"duration"`
			Owner    struct {
				Name string `json:"name"`
			} `json:"owner"`
			Stat struct {
				View int64 `json:"view"`
			} `json:"stat"`
		} `json:"data"`
	}
	err := e.anonClient().do("GET", "https://api.bilibili.com/x/web-interface/view?bvid="+url.QueryEscape(bvid), nil, nil, &r)
	if err != nil {
		return VideoInfo{}, err // 网络错误不缓存
	}
	var info VideoInfo
	switch r.Code {
	case 0:
		info = VideoInfo{BVID: r.Data.BVID, Title: r.Data.Title, Cover: r.Data.Pic, Owner: r.Data.Owner.Name, Duration: r.Data.Duration, Views: r.Data.Stat.View}
	case -404, 62002, 62004, -400:
		err = errVideoNotFound
	default:
		err = fmt.Errorf("查询视频失败：%d %s", r.Code, r.Message)
	}
	e.mu.Lock()
	e.videos[bvid] = videoCache{info: info, err: err, at: time.Now()}
	e.mu.Unlock()
	return info, err
}
