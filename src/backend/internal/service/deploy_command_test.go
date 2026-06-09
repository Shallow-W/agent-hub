package service

import "testing"

func TestIsDeployCommand(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"部署", true},
		{"部署一下", true},
		{"一键部署", true},
		{"发布", true},
		{"deploy", true},
		{"Deploy", true},
		{"/deploy", true},
		{"/deploy now", true},
		{"  部署  ", true},
		{"", false},
		{"帮我写一个网页", false},
		{"这个部署流程怎么搞的，能不能详细说说看看呢", false}, // 长句不误触
		{"你好", false},
	}
	for _, c := range cases {
		if got := isDeployCommand(c.in); got != c.want {
			t.Errorf("isDeployCommand(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
