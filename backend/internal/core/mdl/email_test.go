package mdl

import "testing"

func TestIsValidEmail(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want bool
	}{
		{
			name: "valid address",
			addr: "user@example.com",
			want: true,
		},
		{
			name: "valid address with subdomain",
			addr: "user@mail.example.com",
			want: true,
		},
		{
			name: "display name form rejected",
			addr: "Barry Gibbs <bg@example.com>",
			want: false,
		},
		{
			name: "angle bracket form rejected",
			addr: "<user@example.com>",
			want: false,
		},
		{
			name: "missing at sign",
			addr: "notanemail",
			want: false,
		},
		{
			name: "missing domain",
			addr: "user@",
			want: false,
		},
		{
			name: "empty string",
			addr: "",
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidEmail(tt.addr); got != tt.want {
				t.Errorf("IsValidEmail(%q) = %v, want %v", tt.addr, got, tt.want)
			}
		})
	}
}
