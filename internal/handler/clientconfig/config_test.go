package clientconfig

import "testing"

func TestETagMatches(t *testing.T) {
	const etag = `"cfg-5-1"`

	if !etagMatches(etag, etag) {
		t.Fatal("精确匹配应命中")
	}
	if !etagMatches(`W/`+etag, etag) {
		t.Fatal("弱 ETag 前缀应命中")
	}
	if !etagMatches(`"cfg-5-0", `+etag, etag) {
		t.Fatal("多值应命中")
	}
	if !etagMatches("*", etag) {
		t.Fatal("通配符应命中")
	}
	if etagMatches(`"cfg-5-2"`, etag) {
		t.Fatal("不同版本不应命中")
	}
	if etagMatches("", etag) {
		t.Fatal("空头不应命中")
	}
}
