package generator

import (
	"strings"
	"testing"
)

func TestValidateChineseScript(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		wantErr bool
	}{
		{
			name:    "Chinese narration with English names",
			script:  strings.Repeat("这是一段中文口播，会把 YouTube transcript 消化成故事，而不是直接照读英文材料。", 4),
			wantErr: false,
		},
		{
			name:    "English transcript",
			script:  "This is an English transcript. It keeps going in English and never becomes spoken Chinese.",
			wantErr: true,
		},
		{
			name:    "empty",
			script:  "   ",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateChineseScript(tt.script)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateChineseScript() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
