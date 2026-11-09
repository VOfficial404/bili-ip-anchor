package core

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// 收藏夹的导出和导入：把一个账号自己创建的收藏夹（只含视频）导出成 JSON 文件，
// 再导入到同一个或另一个账号。接口见 bilibili-API-collect 的 fav 部分。

// FavFormat 写在导出文件里，导入时用来认出文件
const FavFormat = "bili-ip-anchor/favorites"

// FavExport 是导出文件的内容
type FavExport struct {
	Format     string      `json:"format"`
	Version    int         `json:"version"`
	ExportedAt time.Time   `json:"exported_at"`
	Mid        string      `json:"mid"`
	Uname      string      `json:"uname"`
	Folders    []FavFolder `json:"folders"`
}

type FavFolder struct {
	Title   string     `json:"title"`
	Intro   string     `json:"intro,omitempty"`
	Private bool       `json:"private,omitempty"`
	Default bool       `json:"default,omitempty"` // 默认收藏夹：导入时放进对方的默认收藏夹，不新建
	Videos  []FavVideo `json:"videos"`            // 和 B 站一样，最近收藏的在前
}

type FavVideo struct {
	Aid     int64     `json:"aid"`
	BVID    string    `json:"bvid"`
	Title   string    `json:"title"`
	Upper   string    `json:"upper,omitempty"`
	FavTime time.Time `json:"fav_time,omitzero"`
	Invalid bool      `json:"invalid,omitempty"` // 已失效（被删除），导入时跳过
}

// FavImport 是导入的进度，放在账号状态里给界面显示
type FavImport struct {
	Running  bool         `json:"running"`
	Folder   string       `json:"folder,omitempty"` // 正在导入的收藏夹
	Total    int          `json:"total"`            // 要处理的视频数（不含已失效的）
	Done     int          `json:"done"`
	Added    int          `json:"added"`
	Skipped  int          `json:"skipped"` // 已经在收藏夹里
	Failed   int          `json:"failed"`
	Failures []FavFailure `json:"failures,omitempty"` // 最多记 50 条
	Created  []string     `json:"created,omitempty"`  // 新建的收藏夹
	Error    string       `json:"error,omitempty"`    // 中途停下的原因
	Started  time.Time    `json:"started"`
	Finished *time.Time   `json:"finished,omitempty"`
}

type FavFailure struct {
	BVID   string `json:"bvid"`
	Title  string `json:"title,omitempty"`
	Reason string `json:"reason"`
}

const maxFavFailures = 50

// favDelay 是导入时两次收藏之间的间隔，太快会被 B 站限制。测试里改成 0。
var favDelay = func() time.Duration { return 1500*time.Millisecond + rand.N(1500*time.Millisecond) }

// favPageDelay 是导出时翻页的间隔
var favPageDelay = func() time.Duration { return 300 * time.Millisecond }

var errBadFavFile = errors.New("不是 bili-ip-anchor 导出的收藏夹文件")

type favFolderInfo struct {
	ID    int64  `json:"id"` // media_id
	Title string `json:"title"`
	Attr  int    `json:"attr"` // 第 0 位：私密；第 1 位：0 是默认收藏夹
	Count int    `json:"media_count"`
}

func (f favFolderInfo) isDefault() bool { return f.Attr&2 == 0 }

// favFolders 列出 mid 自己创建的收藏夹
func favFolders(c *biliClient, mid string) ([]favFolderInfo, error) {
	var r struct {
		apiResp
		Data *struct {
			List []favFolderInfo `json:"list"`
		} `json:"data"`
	}
	if err := c.do("GET", "https://api.bilibili.com/x/v3/fav/folder/created/list-all?up_mid="+url.QueryEscape(mid), nil, nil, &r); err != nil {
		return nil, err
	}
	if err := favErr("列出收藏夹", r.apiResp); err != nil {
		return nil, err
	}
	if r.Data == nil {
		return nil, nil // 一个收藏夹都没有
	}
	return r.Data.List, nil
}

// favErr 把 B 站的错误码变成错误；未登录时返回 errLoggedOut
func favErr(what string, r apiResp) error {
	switch r.Code {
	case 0:
		return nil
	case -101:
		return errLoggedOut
	}
	return fmt.Errorf("%s失败：%d %s", what, r.Code, r.Message)
}

// favFolderVideos 读出一个收藏夹里的全部视频和简介
func (e *Engine) favFolderVideos(c *biliClient, mediaID int64) (string, []FavVideo, error) {
	var intro string
	var videos []FavVideo
	for pn := 1; ; pn++ {
		var r struct {
			apiResp
			Data *struct {
				Info struct {
					Intro string `json:"intro"`
				} `json:"info"`
				Medias []struct {
					ID      int64  `json:"id"`
					Type    int    `json:"type"`
					Title   string `json:"title"`
					Attr    int    `json:"attr"` // 0 正常，其他是已失效
					BVID    string `json:"bvid"`
					FavTime int64  `json:"fav_time"`
					Upper   struct {
						Name string `json:"name"`
					} `json:"upper"`
				} `json:"medias"`
				HasMore bool `json:"has_more"`
			} `json:"data"`
		}
		u := fmt.Sprintf("https://api.bilibili.com/x/v3/fav/resource/list?media_id=%d&pn=%d&ps=20&platform=web", mediaID, pn)
		if err := c.do("GET", u, nil, nil, &r); err != nil {
			return "", nil, err
		}
		if err := favErr("读取收藏夹", r.apiResp); err != nil {
			return "", nil, err
		}
		if r.Data == nil {
			break
		}
		if pn == 1 {
			intro = r.Data.Info.Intro
		}
		for _, m := range r.Data.Medias {
			if m.Type != 2 { // 只要视频，不要音频、合集
				continue
			}
			v := FavVideo{Aid: m.ID, BVID: m.BVID, Title: m.Title, Upper: m.Upper.Name, Invalid: m.Attr != 0}
			if m.FavTime > 0 {
				v.FavTime = time.Unix(m.FavTime, 0)
			}
			videos = append(videos, v)
		}
		if !r.Data.HasMore || len(r.Data.Medias) == 0 {
			break
		}
		if !e.sleep(favPageDelay()) {
			return "", nil, errStopped
		}
	}
	return intro, videos, nil
}

var errStopped = errors.New("程序正在退出")

// ExportFavorites 导出账号自己创建的全部收藏夹
func (e *Engine) ExportFavorites(mid string) (*FavExport, error) {
	acc := e.account(mid)
	if acc == nil {
		return nil, errNoAccount
	}
	cookies, err := e.loadAccountCookies(mid)
	if err != nil {
		return nil, err
	}
	c := e.client(cookies)
	folders, err := favFolders(c, mid)
	if err != nil {
		acc.markLogin(err)
		return nil, err
	}
	out := &FavExport{Format: FavFormat, Version: 1, ExportedAt: time.Now(), Mid: mid, Uname: acc.name(), Folders: []FavFolder{}}
	total := 0
	for _, f := range folders {
		intro, videos, err := e.favFolderVideos(c, f.ID)
		if err != nil {
			return nil, fmt.Errorf("收藏夹「%s」：%w", f.Title, err)
		}
		if videos == nil {
			videos = []FavVideo{}
		}
		out.Folders = append(out.Folders, FavFolder{Title: f.Title, Intro: intro, Private: f.Attr&1 == 1, Default: f.isDefault(), Videos: videos})
		total += len(videos)
	}
	acc.infof("导出了 %d 个收藏夹、%d 个视频", len(out.Folders), total)
	return out, nil
}

// ParseFavExport 检查导入的文件
func ParseFavExport(x *FavExport) error {
	if x.Format != FavFormat || x.Version != 1 {
		return errBadFavFile
	}
	for i, f := range x.Folders {
		if strings.TrimSpace(f.Title) == "" && !f.Default {
			return fmt.Errorf("第 %d 个收藏夹没有名字", i+1)
		}
		for j, v := range f.Videos {
			if v.Aid <= 0 {
				aid, err := BVToAid(v.BVID)
				if err != nil {
					return fmt.Errorf("收藏夹「%s」的第 %d 个视频：%w", f.Title, j+1, err)
				}
				x.Folders[i].Videos[j].Aid = aid
			}
		}
	}
	return nil
}

// ImportFavorites 在后台把 x 里的收藏夹导入账号 mid：按名字放进同名的收藏夹，没有就新建；
// 默认收藏夹放进对方的默认收藏夹；已经收藏的跳过。进度在账号状态的 fav_import 里。
func (e *Engine) ImportFavorites(mid string, x *FavExport) (int, error) {
	acc := e.account(mid)
	if acc == nil {
		return 0, errNoAccount
	}
	if err := ParseFavExport(x); err != nil {
		return 0, err
	}
	cookies, err := e.loadAccountCookies(mid)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, f := range x.Folders {
		for _, v := range f.Videos {
			if !v.Invalid {
				total++
			}
		}
	}
	if !acc.begin("导入收藏") {
		return 0, errBusy
	}
	job := &FavImport{Running: true, Total: total, Started: time.Now()}
	acc.mu.Lock()
	acc.fav = job
	acc.mu.Unlock()
	acc.infof("开始导入 %d 个收藏夹、%d 个视频（来自 %s）", len(x.Folders), total, x.Uname)
	go func() {
		defer acc.end()
		err := e.importFavorites(acc, e.client(cookies), cookies["bili_jct"], x)
		acc.markLogin(err)
		acc.mu.Lock()
		now := time.Now()
		job.Running, job.Folder, job.Finished = false, "", &now
		if err != nil {
			job.Error = err.Error()
		}
		st := *job
		acc.mu.Unlock()
		if err != nil {
			acc.errorf("导入收藏中途停下：%v（已处理 %d/%d）", err, st.Done, st.Total)
		} else {
			acc.infof("导入收藏完成：新增 %d，已有 %d，失败 %d", st.Added, st.Skipped, st.Failed)
		}
	}()
	return total, nil
}

// progress 在锁里修改导入进度
func (acc *account) progress(fn func(j *FavImport)) {
	acc.mu.Lock()
	if acc.fav != nil {
		fn(acc.fav)
	}
	acc.mu.Unlock()
}

func (e *Engine) importFavorites(acc *account, c *biliClient, csrf string, x *FavExport) error {
	folders, err := favFolders(c, acc.mid)
	if err != nil {
		return err
	}
	for _, f := range x.Folders {
		title := strings.TrimSpace(f.Title)
		acc.progress(func(j *FavImport) { j.Folder = title })
		target, created, err := e.favTarget(c, csrf, &folders, f)
		if err != nil {
			return err
		}
		if created {
			acc.progress(func(j *FavImport) { j.Created = append(j.Created, title) })
			acc.infof("新建了收藏夹「%s」", title)
		}
		have, err := favIDs(c, target)
		if err != nil {
			return err
		}
		// 先收藏最早的，导入后的顺序和原来一样
		for _, v := range slices.Backward(f.Videos) {
			if v.Invalid {
				continue
			}
			if have[v.Aid] {
				acc.progress(func(j *FavImport) { j.Done++; j.Skipped++ })
				continue
			}
			if !e.sleep(favDelay()) {
				return errStopped
			}
			err := favAdd(c, csrf, v.Aid, target)
			var fail *favFailErr
			switch {
			case err == nil:
				have[v.Aid] = true
				acc.progress(func(j *FavImport) { j.Done++; j.Added++ })
			case errors.As(err, &fail):
				acc.warnf("收藏 %s 失败：%v", v.BVID, fail.reason)
				acc.progress(func(j *FavImport) {
					j.Done++
					j.Failed++
					if len(j.Failures) < maxFavFailures {
						j.Failures = append(j.Failures, FavFailure{BVID: v.BVID, Title: v.Title, Reason: fail.reason})
					}
				})
			default:
				return err // 未登录、被限制、网络错误：停下
			}
		}
	}
	return nil
}

// favTarget 找到 f 要导入的收藏夹，没有就新建
func (e *Engine) favTarget(c *biliClient, csrf string, folders *[]favFolderInfo, f FavFolder) (int64, bool, error) {
	title := strings.TrimSpace(f.Title)
	for _, t := range *folders {
		if (f.Default && t.isDefault()) || (!f.Default && t.Title == title) {
			return t.ID, false, nil
		}
	}
	if f.Default && title == "" {
		title = "默认收藏夹"
	}
	privacy := "0"
	if f.Private {
		privacy = "1"
	}
	var r struct {
		apiResp
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	form := url.Values{"title": {title}, "intro": {f.Intro}, "privacy": {privacy}, "cover": {""}, "csrf": {csrf}}
	if err := c.do("POST", "https://api.bilibili.com/x/v3/fav/folder/add", form, nil, &r); err != nil {
		return 0, false, err
	}
	if err := favErr("新建收藏夹「"+title+"」", r.apiResp); err != nil {
		return 0, false, err
	}
	*folders = append(*folders, favFolderInfo{ID: r.Data.ID, Title: title, Attr: 2})
	return r.Data.ID, true, nil
}

// favIDs 返回收藏夹里已有视频的 aid
func favIDs(c *biliClient, mediaID int64) (map[int64]bool, error) {
	var r struct {
		apiResp
		Data []struct {
			ID   int64 `json:"id"`
			Type int   `json:"type"`
		} `json:"data"`
	}
	if err := c.do("GET", fmt.Sprintf("https://api.bilibili.com/x/v3/fav/resource/ids?media_id=%d&platform=web", mediaID), nil, nil, &r); err != nil {
		return nil, err
	}
	if err := favErr("读取收藏夹", r.apiResp); err != nil {
		return nil, err
	}
	have := map[int64]bool{}
	for _, d := range r.Data {
		if d.Type == 2 {
			have[d.ID] = true
		}
	}
	return have, nil
}

// favFailErr 是只影响这一个视频的错误（视频不存在、收藏夹满了等），导入继续
type favFailErr struct{ reason string }

func (e *favFailErr) Error() string { return e.reason }

func favAdd(c *biliClient, csrf string, aid, mediaID int64) error {
	var r apiResp
	form := url.Values{
		"rid":           {strconv.FormatInt(aid, 10)},
		"type":          {"2"},
		"add_media_ids": {strconv.FormatInt(mediaID, 10)},
		"del_media_ids": {""},
		"csrf":          {csrf},
	}
	if err := c.do("POST", "https://api.bilibili.com/x/v3/fav/resource/deal", form, nil, &r); err != nil {
		return err
	}
	switch r.Code {
	case 0:
		return nil
	case -101:
		return errLoggedOut
	case -111, -352, -412, -509, -799:
		// CSRF 校验失败、风控、请求太频繁：继续下去也是失败
		return fmt.Errorf("B 站拒绝了收藏请求：%d %s", r.Code, r.Message)
	}
	msg := r.Message
	if msg == "" {
		msg = "错误码 " + strconv.Itoa(r.Code)
	}
	return &favFailErr{reason: msg}
}

// BV 号和 av 号互相换算，见 bilibili-API-collect 的 bvid_desc
const (
	bvAlphabet = "FcwAPNKTMug3GV5Lj7EJnHpWsx4tb8haYeviqBz6rkCy12mUSDQX9RdoZf"
	bvXor      = 23442827791579
	bvMask     = 1<<51 - 1
	bvMax      = 1 << 51
)

// BVToAid 把 BV 号换成 av 号
func BVToAid(bvid string) (int64, error) {
	b := []byte(bvid)
	if len(b) != 12 || !strings.EqualFold(bvid[:2], "BV") {
		return 0, fmt.Errorf("BV 号不对：%q", bvid)
	}
	b[3], b[9] = b[9], b[3]
	b[4], b[7] = b[7], b[4]
	var n int64
	for _, ch := range b[3:] {
		i := strings.IndexByte(bvAlphabet, ch)
		if i < 0 {
			return 0, fmt.Errorf("BV 号不对：%q", bvid)
		}
		n = n*58 + int64(i)
	}
	return (n & bvMask) ^ bvXor, nil
}

// AidToBV 把 av 号换成 BV 号
func AidToBV(aid int64) string {
	b := []byte("BV1000000000")
	n := (bvMax | aid) ^ bvXor
	for i := len(b) - 1; n > 0; i-- {
		b[i] = bvAlphabet[n%58]
		n /= 58
	}
	b[3], b[9] = b[9], b[3]
	b[4], b[7] = b[7], b[4]
	return string(b)
}
