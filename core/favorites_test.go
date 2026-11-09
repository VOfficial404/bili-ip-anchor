package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeMedia struct {
	aid   int64
	title string
	attr  int
	at    int64
}

type fakeFolder struct {
	id     int64
	title  string
	intro  string
	attr   int
	medias []fakeMedia // 最近收藏的在前
}

// fakeFav 模拟 B 站收藏夹接口，按 DedeUserID 区分账号
type fakeFav struct {
	mu      sync.Mutex
	folders map[string][]*fakeFolder
	nextID  int64
	clock   int64
	deny    map[int64]int // 收藏这些 aid 时返回的错误码
}

func newFakeFav() *fakeFav {
	f := &fakeFav{folders: map[string][]*fakeFolder{}, nextID: 100, clock: 1700000000, deny: map[int64]int{}}
	music := &fakeFolder{id: 2, title: "音乐", intro: "好听的", attr: 3}
	for i := range 25 {
		music.medias = append(music.medias, fakeMedia{aid: int64(1000 + i), title: fmt.Sprintf("歌 %d", i), at: 1690000000 - int64(i)})
	}
	f.folders["10001"] = []*fakeFolder{
		{id: 1, title: "默认收藏夹", attr: 0, medias: []fakeMedia{
			{aid: 170001, title: "新的", at: 1690000300},
			{aid: 99, title: "已失效视频", attr: 9, at: 1690000200},
			{aid: 170002, title: "旧的", at: 1690000100},
		}},
		music,
		{id: 3, title: "游戏", attr: 2, medias: []fakeMedia{{aid: 5000, title: "通关", at: 1690000000}, {aid: 5001, title: "下架了", at: 1680000000}}},
	}
	f.folders["10002"] = []*fakeFolder{
		{id: 11, title: "默认收藏夹", attr: 0},
		{id: 12, title: "音乐", attr: 2, medias: []fakeMedia{{aid: 1003, title: "歌 3", at: 1600000000}}},
	}
	f.deny[5001] = 11010
	return f
}

func (f *fakeFav) folder(id int64) *fakeFolder {
	for _, fs := range f.folders {
		for _, d := range fs {
			if d.id == id {
				return d
			}
		}
	}
	return nil
}

func (f *fakeFav) handle(r *http.Request) (*http.Response, bool) {
	if !strings.HasPrefix(r.URL.Path, "/x/v3/fav/") {
		return nil, false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	mid := ""
	if c, err := r.Cookie("DedeUserID"); err == nil {
		mid = c.Value
	}
	q := r.URL.Query()
	r.ParseForm()
	resp := func(v any) (*http.Response, bool) {
		b, _ := json.Marshal(v)
		return jsonResp(string(b)), true
	}
	type m = map[string]any
	switch r.URL.Path {
	case "/x/v3/fav/folder/created/list-all":
		fs := f.folders[q.Get("up_mid")]
		if len(fs) == 0 {
			return resp(m{"code": 0, "data": nil})
		}
		var list []m
		for _, d := range fs {
			list = append(list, m{"id": d.id, "fid": d.id / 100, "mid": q.Get("up_mid"), "attr": d.attr, "title": d.title, "media_count": len(d.medias)})
		}
		return resp(m{"code": 0, "data": m{"count": len(list), "list": list}})
	case "/x/v3/fav/resource/list":
		id, _ := strconv.ParseInt(q.Get("media_id"), 10, 64)
		pn, _ := strconv.Atoi(q.Get("pn"))
		ps, _ := strconv.Atoi(q.Get("ps"))
		d := f.folder(id)
		if d == nil {
			return resp(m{"code": -400, "message": "请求错误"})
		}
		lo, hi := min((pn-1)*ps, len(d.medias)), min(pn*ps, len(d.medias))
		var medias []m
		for _, x := range d.medias[lo:hi] {
			medias = append(medias, m{"id": x.aid, "type": 2, "title": x.title, "attr": x.attr, "bvid": AidToBV(x.aid), "bv_id": AidToBV(x.aid), "fav_time": x.at, "upper": m{"mid": 1, "name": "UP"}})
		}
		medias = append(medias, m{"id": 7, "type": 12, "title": "音频不导出"})
		return resp(m{"code": 0, "data": m{"info": m{"id": id, "title": d.title, "intro": d.intro, "attr": d.attr}, "medias": medias, "has_more": hi < len(d.medias)}})
	case "/x/v3/fav/resource/ids":
		id, _ := strconv.ParseInt(q.Get("media_id"), 10, 64)
		var ids []m
		for _, x := range f.folder(id).medias {
			ids = append(ids, m{"id": x.aid, "type": 2, "bv_id": AidToBV(x.aid)})
		}
		return resp(m{"code": 0, "data": ids})
	case "/x/v3/fav/folder/add":
		if r.PostForm.Get("csrf") != "j" {
			return resp(m{"code": -111, "message": "csrf 校验失败"})
		}
		f.nextID++
		attr := 2
		if r.PostForm.Get("privacy") == "1" {
			attr = 3
		}
		f.folders[mid] = append(f.folders[mid], &fakeFolder{id: f.nextID, title: r.PostForm.Get("title"), intro: r.PostForm.Get("intro"), attr: attr})
		return resp(m{"code": 0, "data": m{"id": f.nextID}})
	case "/x/v3/fav/resource/deal":
		if r.Method != "POST" || r.PostForm.Get("csrf") != "j" || r.PostForm.Get("type") != "2" {
			return resp(m{"code": -111, "message": "csrf 校验失败"})
		}
		aid, _ := strconv.ParseInt(r.PostForm.Get("rid"), 10, 64)
		if code := f.deny[aid]; code != 0 {
			return resp(m{"code": code, "message": "视频不见了"})
		}
		id, _ := strconv.ParseInt(r.PostForm.Get("add_media_ids"), 10, 64)
		d := f.folder(id)
		f.clock++
		d.medias = slices.Insert(d.medias, 0, fakeMedia{aid: aid, title: "收藏的", at: f.clock})
		return resp(m{"code": 0})
	}
	return resp(m{"code": -404, "message": "啥都木有"})
}

func newFavEngine(t *testing.T) (*Engine, *fakeFav) {
	t.Helper()
	e := New(Config{DataDir: t.TempDir(), Version: "test"})
	fav := newFakeFav()
	e.cfg.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if resp, ok := fav.handle(r); ok {
			return resp, nil
		}
		t.Errorf("意外的请求：%s", r.URL)
		return jsonResp(`{"code":-404}`), nil
	})
	favDelay = func() time.Duration { return 0 }
	favPageDelay = func() time.Duration { return 0 }
	for _, mid := range []string{"10001", "10002"} {
		if _, err := e.saveAccountCookies(map[string]string{"SESSDATA": "s", "bili_jct": "j", "DedeUserID": mid}); err != nil {
			t.Fatal(err)
		}
		e.accounts[mid] = e.newAccount(mid)
	}
	return e, fav
}

func TestBVToAid(t *testing.T) {
	for _, c := range []struct {
		aid  int64
		bvid string
	}{{170001, "BV17x411w7KC"}, {111298867365120, "BV1L9Uoa9EUx"}, {1, "BV1xx411c7mQ"}} {
		if got := AidToBV(c.aid); got != c.bvid {
			t.Errorf("av%d 应该是 %s，得到 %s", c.aid, c.bvid, got)
		}
		if got, err := BVToAid(c.bvid); err != nil || got != c.aid {
			t.Errorf("%s 应该是 av%d，得到 %d %v", c.bvid, c.aid, got, err)
		}
	}
	for _, bad := range []string{"", "BV17x411w7K", "BV17x411w7K0", "av170001", "BV17x411w7KCC"} {
		if _, err := BVToAid(bad); err == nil {
			t.Errorf("%q 应该报错", bad)
		}
	}
}

func TestExportFavorites(t *testing.T) {
	e, _ := newFavEngine(t)
	x, err := e.ExportFavorites("10001")
	if err != nil {
		t.Fatal(err)
	}
	if x.Format != FavFormat || x.Mid != "10001" || len(x.Folders) != 3 {
		t.Fatalf("导出的文件不对：%+v", x)
	}
	def, music := x.Folders[0], x.Folders[1]
	if !def.Default || def.Private || len(def.Videos) != 3 || !def.Videos[1].Invalid || def.Videos[0].BVID != "BV17x411w7KC" {
		t.Errorf("默认收藏夹不对：%+v", def)
	}
	if music.Default || !music.Private || music.Intro != "好听的" || len(music.Videos) != 25 {
		t.Errorf("翻页或私密不对：%d 个视频，%+v", len(music.Videos), music.Title)
	}
	if music.Videos[24].Aid != 1024 || music.Videos[0].FavTime.Unix() != 1690000000 {
		t.Errorf("视频顺序或时间不对：%+v", music.Videos[0])
	}
	// 写成 JSON 再读回来也能导入
	b, _ := json.Marshal(x)
	var back FavExport
	if err := json.Unmarshal(b, &back); err != nil || ParseFavExport(&back) != nil {
		t.Fatalf("读不回来：%v", err)
	}
}

func waitFavImport(t *testing.T, acc *account) FavImport {
	t.Helper()
	for range 500 {
		if st := acc.status(); st.Fav != nil && !st.Fav.Running {
			if st.Busy != "" {
				continue // end() 还没调用
			}
			return *st.Fav
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("导入没有结束")
	return FavImport{}
}

func TestImportFavorites(t *testing.T) {
	e, fav := newFavEngine(t)
	x, err := e.ExportFavorites("10001")
	if err != nil {
		t.Fatal(err)
	}
	total, err := e.ImportFavorites("10002", x)
	if err != nil || total != 29 { // 3+25+2，减去 1 个已失效的
		t.Fatalf("开始导入：%d %v", total, err)
	}
	if _, err := e.ImportFavorites("10002", x); err != errBusy {
		t.Errorf("导入中再导入应该返回 errBusy，得到 %v", err)
	}
	job := waitFavImport(t, e.account("10002"))
	if job.Error != "" || job.Done != 29 || job.Added != 27 || job.Skipped != 1 || job.Failed != 1 {
		t.Fatalf("导入结果不对：%+v", job)
	}
	if len(job.Failures) != 1 || job.Failures[0].BVID != AidToBV(5001) || job.Failures[0].Reason != "视频不见了" {
		t.Errorf("失败记录不对：%+v", job.Failures)
	}
	if !slices.Equal(job.Created, []string{"游戏"}) {
		t.Errorf("应该只新建「游戏」，得到 %v", job.Created)
	}

	fav.mu.Lock()
	defer fav.mu.Unlock()
	got := fav.folders["10002"]
	if len(got) != 3 || got[2].title != "游戏" || got[2].attr != 2 {
		t.Fatalf("收藏夹不对：%+v", got)
	}
	var aids []int64
	for _, m := range got[0].medias {
		aids = append(aids, m.aid)
	}
	if !slices.Equal(aids, []int64{170001, 170002}) {
		t.Errorf("默认收藏夹的顺序应该和原来一样，得到 %v", aids)
	}
	if n := len(got[1].medias); n != 25 {
		t.Errorf("音乐收藏夹应该有 25 个，得到 %d", n)
	}
}

// 被风控时停下，不继续一个个失败
func TestImportFavoritesStopsOnRiskControl(t *testing.T) {
	e, fav := newFavEngine(t)
	fav.deny[1010] = -412
	x, err := e.ExportFavorites("10001")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.ImportFavorites("10002", x); err != nil {
		t.Fatal(err)
	}
	job := waitFavImport(t, e.account("10002"))
	if !strings.Contains(job.Error, "-412") || job.Done >= job.Total || job.Failed != 0 {
		t.Errorf("应该在 -412 时停下：%+v", job)
	}
}

func TestFavoritesHTTP(t *testing.T) {
	e, _ := newFavEngine(t)
	e.cfg.Password = "pw"
	srv := httptest.NewServer(e.Handler())
	defer srv.Close()
	c := srv.Client()
	login, _ := http.NewRequest("POST", srv.URL+"/api/session", strings.NewReader(`{"password":"pw"}`))
	login.Header.Set("X-Requested-With", "bia")
	resp, err := c.Do(login)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("登录：%v %v", resp.Status, err)
	}
	cookies := resp.Cookies()
	do := func(method, path string, body []byte) *http.Response {
		req, _ := http.NewRequest(method, srv.URL+path, bytes.NewReader(body))
		req.Header.Set("X-Requested-With", "bia")
		for _, ck := range cookies {
			req.AddCookie(ck)
		}
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	resp = do("GET", "/api/accounts/10001/favorites", nil)
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Disposition"), "bili-favorites-10001-") {
		t.Fatalf("导出：%s %v", resp.Status, resp.Header)
	}
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	file := buf.Bytes()

	if resp := do("POST", "/api/accounts/10002/favorites", []byte(`{"format":"别的"}`)); resp.StatusCode != 400 {
		t.Errorf("别的文件应该返回 400，得到 %s", resp.Status)
	}
	if resp := do("POST", "/api/accounts/404/favorites", file); resp.StatusCode != 404 {
		t.Errorf("不存在的账号应该返回 404，得到 %s", resp.Status)
	}
	resp = do("POST", "/api/accounts/10002/favorites", file)
	var r struct{ Total int }
	json.NewDecoder(resp.Body).Decode(&r)
	if resp.StatusCode != 202 || r.Total != 29 {
		t.Fatalf("导入：%s %+v", resp.Status, r)
	}
	waitFavImport(t, e.account("10002"))
}
