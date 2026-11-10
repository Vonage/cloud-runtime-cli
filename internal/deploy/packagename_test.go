package deploy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPackageNameIsAValidImageName(t *testing.T) {
	tests := []struct {
		project, instance, want string
	}{
		{"web", "prod", "web-prod"},
		{"My_Shop 2", "Dev.EU", "my-shop-2-dev-eu"},
		{"--a--", "b", "a-b"},
		{"123", "456", "123-456"},
		{"café", "über", "caf-ber"},
		{"__", "..", ""},
	}
	for _, tt := range tests {
		t.Run(tt.project+"/"+tt.instance, func(t *testing.T) {
			assert.Equal(t, tt.want, packageName(tt.project, tt.instance))
		})
	}
}
