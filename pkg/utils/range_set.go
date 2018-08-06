/*
 * Copyright 2018 Haines Chan
 *
 * This program is free software; you can redistribute and/or modify it
 * under the terms of the standard MIT license. See LICENSE for more details
 */

package utils

import (
	"fmt"
	"github.com/containernetworking/plugins/pkg/ip"
	"net"
	"sort"
	"strings"
)

// RangeSet is a array of Range
type RangeSet []Range

// RangeIter is a iterator of a RangeSet
type RangeIter struct {
	rangeset *RangeSet

	// The current range id
	rangeIdx int

	// Our current position
	cur net.IP

	// The IP and range index where we started iterating; if we hit this again, we're done.
	startIP    net.IP
	startRange int
}

// Next returns the next IP, its mask, and its gateway. Returns nil
// if the iterator has been exhausted
func (i *RangeIter) Next() (*net.IPNet, net.IP) {
	r := (*i.rangeset)[i.rangeIdx]

	// If this is the first time iterating and we're not starting in the middle
	// of the range, then start at rangeStart, which is inclusive
	if i.cur == nil {
		i.cur = r.RangeStart
		i.startIP = i.cur
		if i.cur.Equal(r.Gateway) {
			return i.Next()
		}
		return &net.IPNet{IP: i.cur, Mask: r.Subnet.Mask}, r.Gateway
	}

	// If we've reached the end of this range, we need to advance the range
	// RangeEnd is inclusive as well
	if i.cur.Equal(r.RangeEnd) {
		i.rangeIdx++
		i.rangeIdx %= len(*i.rangeset)
		r = (*i.rangeset)[i.rangeIdx]

		i.cur = r.RangeStart
	} else {
		i.cur = ip.NextIP(i.cur)
	}

	if i.startIP == nil {
		i.startIP = i.cur
	} else if i.rangeIdx == i.startRange && i.cur.Equal(i.startIP) {
		// IF we've looped back to where we started, give up
		return nil, nil
	}

	if i.cur.Equal(r.Gateway) {
		return i.Next()
	}

	return &net.IPNet{IP: i.cur, Mask: r.Subnet.Mask}, r.Gateway
}

// Contains returns true if any range in this set contains an IP
func (rs *RangeSet) Contains(addr net.IP) bool {
	r, _ := rs.RangeFor(addr)
	return r != nil
}

// RangeFor finds the range that contains an IP, or nil if not found
func (rs *RangeSet) RangeFor(addr net.IP) (*Range, error) {
	if err := canonicalizeIP(&addr); err != nil {
		return nil, err
	}

	for _, r := range *rs {
		if r.Contains(addr) {
			return &r, nil
		}
	}

	return nil, fmt.Errorf("%s not in range set %s", addr.String(), rs.String())
}

// Overlaps returns true if any ranges in any set overlap with this one
func (rs *RangeSet) Overlaps(p1 *RangeSet) bool {
	for _, r := range *rs {
		for _, r1 := range *p1 {
			if r.Overlaps(&r1) {
				return true
			}
		}
	}
	return false
}

// Canonicalize ensures the RangeSet is in a standard form, and detects any
// invalid input. Call Range.Canonicalize() on every Range in the set
func (rs *RangeSet) Canonicalize() error {
	if len(*rs) == 0 {
		return fmt.Errorf("empty range set")
	}

	fam := 0
	for i := range *rs {
		if err := (*rs)[i].Canonicalize(); err != nil {
			return err
		}
		if i == 0 {
			fam = len((*rs)[i].RangeStart)
		} else {
			if fam != len((*rs)[i].RangeStart) {
				return fmt.Errorf("mixed address families")
			}
		}
	}

	// Make sure none of the ranges in the set overlap
	l := len(*rs)
	for i, r1 := range (*rs)[:l-1] {
		for _, r2 := range (*rs)[i+1:] {
			if r1.Overlaps(&r2) {
				return fmt.Errorf("subnets %s and %s overlap", r1.String(), r2.String())
			}
		}
	}

	return nil
}

func (rs *RangeSet) String() string {
	out := []string{}
	for _, r := range *rs {
		out = append(out, r.String())
	}

	return strings.Join(out, ",")
}

// Concat concats RangeSet from string for given subnet.
// eg: "10.0.0.[2-4], 10.0.1.4, 10.0.1.5, 10.0.1.9", 10.0.1.0/24
func (rs *RangeSet) Concat(s string, subnet *net.IPNet) (*RangeSet, error) {
	// TODO: the code is designed for ipv4, what will happens when ipv6?
	// No special case when s is empty or rs is empty.
	if s == "" || strings.TrimSpace(s) == "" {
		return rs, nil
	}
	ranges := strings.Split(s, ",")
	for _, r := range ranges {
		// Remove all lead blanks and tailed blanks.
		r = strings.TrimSpace(r)
		if strings.HasSuffix(r, "]") {
			// eg: ["10.0.1.", "4-8"]
			segments := strings.Split(strings.TrimSuffix(r, "]"), "[")
			suffixs := strings.Split(segments[1], "-")
			start := net.ParseIP(segments[0] + suffixs[0])
			if !subnet.Contains(start) {
				// This range don't belong to the subnet, so continue here.
				continue
			}
			end := net.ParseIP(segments[0] + suffixs[1])
			if start == nil || end == nil {
				return nil, fmt.Errorf("invalid IP ranges %s", r)
			}

			*rs = append(*rs, Range{
				RangeStart: start,
				RangeEnd:   end,
				Subnet:     *subnet,
			})
		} else {
			// eg: 10.1.8.9
			current := net.ParseIP(r)
			if current == nil {
				return nil, fmt.Errorf("invalid IP ranges %s", r)
			}
			if !subnet.Contains(current) {
				// This range don't belong to the subnet, so continue here.
				continue
			}
			*rs = append(*rs, Range{
				RangeStart: current,
				RangeEnd:   current,
				Subnet:     *subnet,
			})
		}
	}
	sort.Sort(rs)

	cursor := 0
	l := rs.Len()
	i := 1
	for ; i < l; i++ {
		// A gap here
		if ip.Cmp((*rs)[cursor].RangeEnd, (*rs)[i].RangeStart) < 0 && !(*rs)[i].RangeStart.Equal(ip.NextIP((*rs)[cursor].RangeEnd)) {
			if i > cursor+1 {
				*rs = append((*rs)[:cursor+1], (*rs)[i:]...)
				gap := i - cursor - 1
				l -= gap
				i -= gap

			}
			cursor++
		} else { // overlap case
			if ip.Cmp((*rs)[i].RangeEnd, (*rs)[cursor].RangeEnd) > 0 {
				(*rs)[cursor].RangeEnd = (*rs)[i].RangeEnd
			}
		}
	}

	if i > cursor+1 {
		*rs = append((*rs)[:cursor+1], (*rs)[i:]...)
	}
	return rs, nil
}

func (rs RangeSet) Len() int {
	return len(rs)
}

func (rs RangeSet) Swap(i, j int) {
	rs[i], rs[j] = rs[j], rs[i]
}

func (rs RangeSet) Less(i, j int) bool {
	a, b := rs[i], rs[j]

	if ip.Cmp(a.RangeStart, b.RangeStart) != 0 {
		return ip.Cmp(a.RangeStart, b.RangeStart) < 0
	}
	return ip.Cmp(a.RangeEnd, b.RangeEnd) < 0
}
