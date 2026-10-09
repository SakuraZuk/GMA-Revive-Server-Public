package hotfix

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func load(t *testing.T, c *Catalog, path string, r Release) error {
	t.Helper()
	body, _ := json.Marshal(r)
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	return c.Load(path)
}

func TestReleaseChannelsAndMonotonicIndex(t *testing.T) {
	var c Catalog
	path := filepath.Join(t.TempDir(), "hotfix.json")
	r := Release{StartupScripts: map[string]string{"1.0.125": "pass\n"}, Runtime: Runtime{Index: 2, Script: "pass\n"}}
	if err := load(t, &c, path, r); err != nil {
		t.Fatal(err)
	}
	body, err := base64.StdEncoding.DecodeString(c.Startup())
	if err != nil || !strings.Contains(string(body), "1.0.125") {
		t.Fatal("启动期版本未正确编码")
	}
	if c.Query(0) != r.Runtime {
		t.Fatal("新热修未下发")
	}
	for _, index := range []int{2, 3} {
		if got := c.Query(index); got.Script != "" || got.Index != index {
			t.Fatal("无更新应保留客户端索引")
		}
	}
	r.Runtime.Script = "different\n"
	if err := load(t, &c, path, r); err == nil {
		t.Fatal("同号换代码未拒绝")
	}
	r.Runtime.Index = 1
	if err := load(t, &c, path, r); err == nil {
		t.Fatal("索引回退未拒绝")
	}
	if c.Query(0).Script != "pass\n" {
		t.Fatal("无效发布污染快照")
	}
}

func TestHTTPManifestRangeAndUnknownRoute(t *testing.T) {
	dir := t.TempDir()
	var c Catalog
	if err := load(t, &c, filepath.Join(dir, "hotfix.json"), Release{StartupScripts: map[string]string{}, Runtime: Runtime{}}); err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{"android":{"version":"1.0.128","base_version":"1.0.8","npk":{},"0patchpath":"https://192.0.2.20/resources/"}}`)
	for name, body := range map[string][]byte{"patch_list_pub_android.txt": manifest, "notice.xml": []byte("<root/>")} {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "resources"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "resources", "test.npk"), []byte("1234567890"), 0600); err != nil {
		t.Fatal(err)
	}
	handler, err := Handler(&c, dir, "127.0.0.1:9000", "127.0.0.1", "r18sex.net", "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/resources/test.npk", nil)
	request.Header.Set("Range", "bytes=2-5")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	if w.Code != 206 || w.Body.String() != "3456" {
		t.Fatalf("资源续传错误：%d %s", w.Code, w.Body)
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/server_list_public.txt?123", nil))
	if !strings.Contains(w.Body.String(), " 明日生机 ") || strings.Count(w.Body.String(), "127.0.0.1:9000 ") != 4 || !strings.Contains(w.Body.String(), "network=bgp") {
		t.Fatal("服务器列表格式错误")
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/h62/server_list_public.txt", nil))
	if !strings.Contains(w.Body.String(), " 明日生机 ") {
		t.Fatal("项目前缀 /h62/ 未被剥离")
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/unknown", nil))
	// 2026-10-05 实测修订：SDK 域(如 analytics.mpay)对 404 弹网络错误窗并死循环重试，
	// 未知路径须返回空 200（{}）；根路径仍 404。
	if w.Code != 200 || w.Body.String() != "{}" {
		t.Fatalf("未知路径应返回空 200：Got %d %q", w.Code, w.Body.String())
	}
	if err := os.WriteFile(filepath.Join(dir, "patch_list_pub_android.txt"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Handler(&c, dir, "127.0.0.1:9000", "127.0.0.1", "r18sex.net", "192.0.2.10"); err == nil {
		t.Fatal("空补丁清单未被拒绝")
	}
}

func dnsQuery(name string, qtype uint16) []byte {
	query := []byte{0x12, 0x34, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0}
	for _, label := range strings.Split(name, ".") {
		query = append(query, byte(len(label)))
		query = append(query, label...)
	}
	return append(query, 0, byte(qtype>>8), byte(qtype), 0, 1)
}

func TestDNSProjectDomainsAndNoRecursion(t *testing.T) {
	for _, name := range []string{"h62.update.netease.com", "applog.nie.netease.com", "appdump.easebar.com"} {
		r, err := DNSReply(dnsQuery(name, 1), net.ParseIP("192.0.2.20"))
		if err != nil || binary.BigEndian.Uint16(r[6:8]) != 1 || net.IP(r[len(r)-4:]).String() != "192.0.2.20" {
			t.Fatalf("%s 解析错误", name)
		}
	}
	r, err := DNSReply(dnsQuery("h62.update.netease.com", 28), net.ParseIP("192.0.2.20"))
	if err != nil || binary.BigEndian.Uint16(r[6:8]) != 0 || r[3] != 0 {
		t.Fatal("IPv6 查询应返回无记录")
	}
	r, err = DNSReply(dnsQuery("fakenetease.com", 1), net.ParseIP("192.0.2.20"))
	if err != nil || r[3]&15 != 5 {
		t.Fatal("非项目域名必须拒绝递归")
	}
	if _, err := DNSReply([]byte{1}, net.ParseIP("192.0.2.20")); err == nil {
		t.Fatal("截断查询未拒绝")
	}
}

func TestDNSGameDomainMapping(t *testing.T) {
	r, err := DNSReplyForDomains(dnsQuery("login.r18sex.net", 1), map[string]net.IP{
		"r18sex.net":  net.ParseIP("192.0.2.10"),
		"netease.com": net.ParseIP("192.0.2.20"),
	})
	if err != nil || net.IP(r[len(r)-4:]).String() != "192.0.2.10" {
		t.Fatalf("游戏域名解析错误：%v", err)
	}
}
