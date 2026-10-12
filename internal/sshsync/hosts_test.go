package sshsync

import (
	"reflect"
	"testing"
)

func TestServersKeepsSingleIPv4Only(t *testing.T) {
	got := Servers(map[string]string{
		"app-1":      "192.0.2.10",
		"db":         "198.51.100.5/32",
		"office-lan": "10.0.0.0/24",
		"v6":         "fd00::1",
		"junk":       "not-an-ip",
	})
	want := []Server{{Name: "app-1", IP: "192.0.2.10"}, {Name: "db", IP: "198.51.100.5"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Servers = %+v, want %+v", got, want)
	}
}
