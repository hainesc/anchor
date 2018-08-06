/*
 * Copyright 2018 Haines Chan
 *
 * This program is free software; you can redistribute and/or modify it
 * under the terms of the standard MIT license. See LICENSE for more details
 */

package app

import (
	"fmt"
	"github.com/vishvananda/netlink"
)

// ModeFromString configures different mode for macvlan
func ModeFromString(s string) (netlink.MacvlanMode, error) {
	switch s {
	case "", "bridge":
		return netlink.MACVLAN_MODE_BRIDGE, nil
	case "private":
		return netlink.MACVLAN_MODE_PRIVATE, nil
	case "vepa":
		return netlink.MACVLAN_MODE_VEPA, nil
	case "passthru":
		return netlink.MACVLAN_MODE_PASSTHRU, nil
	default:
		return 0, fmt.Errorf("unknown macvlan mode: %q", s)
	}
}
