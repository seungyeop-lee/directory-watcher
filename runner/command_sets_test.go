package runner

import "testing"

func TestPath_IsSubFolder(t *testing.T) {
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
				input: Path("./dir1/tmp/tmp2"),
			},
			want: true,
		},
		{
			name: "서브 폴더가 아닌경우",
			p:    "./dir1",
			args: args{
				input: Path("./dir2/tmp/tmp2"),
			},
			want: false,
		},
		{
			name: "서브 폴더가 아닌경우 (하지만 문자열로는 포함됨)",
			p:    "./dir1",
			args: args{
				input: Path("./dir2/dir1/tmp/tmp2"),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.IsSubFolder(tt.args.input); got != tt.want {
				t.Errorf("IsSubFolder() = %v, want %v", got, tt.want)
			}
		})
	}
}
