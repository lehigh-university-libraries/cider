package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCIDR(t *testing.T) {
	ips := map[string]string{
		"127.0.0.1":                              "127.0.0.0/16",
		"8.8.8.8":                                "8.8.0.0/16",
		"2001:db8:3333:4444:5555:6666:7777:8888": "2001:db8:3333:4444::/64",
	}
	for ip, expectedCidr := range ips {
		cidr, err := GetCIDR(ip)
		assert.Equal(t, nil, err)
		assert.Equal(t, expectedCidr, cidr)
	}

}
