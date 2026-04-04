package toolkit

import "testing"

func TestFixWindowsStartEmptyTitle(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "start /B without title",
			in:   `start /B cmd /c "echo hi"`,
			want: `start "" /B cmd /c "echo hi"`,
		},
		{
			name: "start with spaces before switch",
			in:   `start   /MIN notepad`,
			want: `start   "" /MIN notepad`,
		},
		{
			name: "already has empty title",
			in:   `start "" /B cmd /c "echo hi"`,
			want: `start "" /B cmd /c "echo hi"`,
		},
		{
			name: "nested cmd /c start /B",
			in:   `cmd /c start /B java -jar app.jar`,
			want: `cmd /c start "" /B java -jar app.jar`,
		},
		{
			name: "no start",
			in:   `java -version`,
			want: `java -version`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixWindowsStartEmptyTitle(tt.in)
			if got != tt.want {
				t.Errorf("fixWindowsStartEmptyTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}
