package handlers

import "testing"

func TestSlugify(t *testing.T) {
	tests := map[string]string{
		"Hello Echo": "hello-echo",
		"Học Golang cơ bản – Đường đi": "hoc-golang-co-ban-duong-di",
		"ĐÂY LÀ TIÊU ĐỀ":               "day-la-tieu-de",
		"  --Go & GORM!!  ":            "go-gorm",
		"C++ / C#":                     "c-c",
		"!!!":                          "article",
		"":                             "article",
	}
	for in, want := range tests {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
