package domain

import (
	"testing"
)

func TestPath_Contains(t *testing.T) {
	type args struct {
		input Path
	}
	tests := []struct {
		name string
		p    Path
		args args
		want bool
	}{
		{
			name: "서브 폴더일 경우",
			p:    "./dir1",
			args: args{
				input: "./dir1/tmp/tmp2",
			},
			want: true,
		},
		{
			name: "서브 폴더가 아닌경우",
			p:    "./dir1",
			args: args{
				input: "./dir2/tmp/tmp2",
			},
			want: false,
		},
		{
			name: "서브 폴더가 아닌경우 (하지만 문자열로는 포함됨)",
			p:    "./dir1",
			args: args{
				input: "./dir2/dir1/tmp/tmp2",
			},
			want: false,
		},
		{
			name: "서브 폴더가 아닌경우 (이름 앞부분만 같음)",
			p:    "./dir1",
			args: args{
				input: "./dir10",
			},
			want: false,
		},
		{
			name: "서브 폴더가 아닌경우 (이름 앞부분만 같고 뒤에 글자가 더 붙음)",
			p:    "./dir1",
			args: args{
				input: "./dir1-old/tmp",
			},
			want: false,
		},
		{
			name: "서브 폴더일 경우 (이름이 ..으로 시작하는 하위 폴더)",
			p:    "./dir1",
			args: args{
				input: "./dir1/..tmp",
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Contains(tt.args.input); got != tt.want {
				t.Errorf("Contains() = %v, want %v", got, tt.want)
			}
		})
	}
}
