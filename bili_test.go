package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func jsonResp(s string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(s)), Header: http.Header{}}
}

func TestPostImageDynamic(t *testing.T) {
	cfg.DataDir = t.TempDir() // 没有覆盖图片，应使用内置图片
	removeDelay = func() time.Duration { return 0 }

	var uploaded []byte
	var createBody map[string]any
	removed := false
	c := newClient(map[string]string{"SESSDATA": "s", "bili_jct": "tok"})
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/x/dynamic/feed/draw/upload_bfs":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatal(err)
			}
			if r.FormValue("biz") != "new_dyn" || r.FormValue("csrf") != "tok" {
				t.Errorf("上传表单字段不对：%v", r.MultipartForm.Value)
			}
			f, _, err := r.FormFile("file_up")
			if err != nil {
				t.Fatal(err)
			}
			uploaded, _ = io.ReadAll(f)
			return jsonResp(`{"code":0,"data":{"image_url":"https://i0.hdslb.com/x.jpg","image_width":687,"image_height":755,"img_size":68.0}}`), nil
		case "/x/dynamic/feed/create/dyn":
			json.NewDecoder(r.Body).Decode(&createBody)
			return jsonResp(`{"code":0,"data":{"dyn_id_str":"123"}}`), nil
		case "/x/dynamic/feed/operate/remove":
			removed = true
			return jsonResp(`{"code":0}`), nil
		}
		t.Fatalf("意外的请求：%s", r.URL)
		return nil, nil
	})

	if err := postAndRemoveDynamic(c, "tok", t.Logf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(uploaded, defaultDynamicImage) || len(uploaded) == 0 {
		t.Errorf("上传的不是内置图片（%d 字节）", len(uploaded))
	}
	req := createBody["dyn_req"].(map[string]any)
	pics := req["pics"].([]any)
	if req["scene"].(float64) != 2 || len(pics) != 1 || pics[0].(map[string]any)["img_src"] != "https://i0.hdslb.com/x.jpg" {
		t.Errorf("发动态请求不对：%v", req)
	}
	if !removed {
		t.Error("没有删除动态")
	}
}
