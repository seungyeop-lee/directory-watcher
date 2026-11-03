package domain

import (
	"path/filepath"
	"testing"
)

func TestPath_IsSubDirLogical(t *testing.T) {
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
			if got := tt.p.isSubDirLogical(tt.args.input); got != tt.want {
				t.Errorf("IsSubDir() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPath_Rebase(t *testing.T) {
	tests := []struct {
		name string
		p    Path
		want Path
	}{
		{name: "from과 같은 경로", p: "./link", want: Path(filepath.FromSlash("/real"))},
		{name: "from 아래 경로", p: "./link/sub/deep", want: Path(filepath.FromSlash("/real/sub/deep"))},
		{name: "from 밖 경로 (이름 앞부분만 같음)", p: "./link2/sub", want: "./link2/sub"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Rebase("./link", "/real"); got != tt.want {
				t.Errorf("Rebase() = %v, want %v", got, tt.want)
			}
		})
	}
}
