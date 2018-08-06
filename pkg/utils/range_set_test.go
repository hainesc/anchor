/*
 * Copyright 2018 Haines Chan
 *
 * This program is free software; you can redistribute and/or modify it
 * under the terms of the standard MIT license. See LICENSE for more details
 */

package utils

import (
	"net"
	"strings"
	"testing"
)

func Test_Contat(t *testing.T) {
	_, subnet, _ := net.ParseCIDR("10.0.1.0/24")

	s := make([]string, 0)
	origin := RangeSet{}
	t.Log("testing empty string")
	if _, err := origin.Concat("", subnet); err != nil {
		t.Fatal(err.Error())
	}
	t.Log("testing empty map")
	if _, err := origin.Concat(strings.Join(s, ","), subnet); err != nil {

		t.Fatal(err.Error())
	}
	t.Log("test succuss")
}
