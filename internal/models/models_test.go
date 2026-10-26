package models

import "testing"

func TestComponentName(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{id: "go://golang.org/x/net:0.59.0", want: "golang.org/x/net:0.59.0"},
		{id: "rpm://7:openssl-libs:3.0.7", want: "7:openssl-libs:3.0.7"},
		{id: "golang.org/x/net:0.59.0", want: "golang.org/x/net:0.59.0"},
		{id: "", want: ""},
	}
	for _, tt := range tests {
		if got := ComponentName(tt.id); got != tt.want {
			t.Errorf("ComponentName(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
}
