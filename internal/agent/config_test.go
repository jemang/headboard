package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigAndValidate(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config")
	os.WriteFile(p, []byte("url=https://hb.example\ntoken=hbk_abc\n"), 0o600)

	c, err := LoadConfig(p)
	if err != nil || c.URL != "https://hb.example" || c.Token != "hbk_abc" {
		t.Fatalf("LoadConfig = %+v, %v", c, err)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}

	for _, bad := range []Config{
		{URL: "http://hb.example", Token: "hbk_abc"},
		{URL: "http://100.64.0.1", Token: "hbk_abc"},
		{URL: "https://hb.example", Token: "abc"},
		{URL: "", Token: "hbk_abc"},
	} {
		if bad.Validate() == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}
