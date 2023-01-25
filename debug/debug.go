package debug

import "strings"

type Info struct {
	lines []string
}

func (info *Info) Add(v string) *Info {
	info.lines = append(info.lines, v)
	return info
}

func (info *Info) AddAll(i *Info) *Info {
	info.lines = append(info.lines, i.lines...)
	return info
}

func (info *Info) String() string {
	return strings.Join(info.lines, "\n")
}
