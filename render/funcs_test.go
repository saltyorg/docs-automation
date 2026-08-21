package render

import "testing"

func TestFormatDescriptionComment(t *testing.T) {
	tests := []struct {
		name        string
		description string
		want        string
	}{
		{
			name:        "single line",
			description: "Enable GPU access",
			want:        "# Enable GPU access",
		},
		{
			name:        "multiple lines",
			description: "Enable GPU access.\nRequires server support.",
			want:        "# Enable GPU access.\n# Requires server support.",
		},
		{
			name:        "blank line",
			description: "First paragraph.\n\nSecond paragraph.",
			want:        "# First paragraph.\n#\n# Second paragraph.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatDescriptionComment(tt.description); got != tt.want {
				t.Fatalf("formatDescriptionComment() = %q, want %q", got, tt.want)
			}
		})
	}
}
