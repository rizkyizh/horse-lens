package workspace

import (
	"github.com/rizkyizh/horse-lens/internal/config"
)

type Link struct {
	Src   string
	Alias string
}

type Workspace struct {
	Name  string
	Links []Link
}

func FromProfile(p config.Profile) Workspace {
	ws := Workspace{Name: p.Name}
	for _, l := range p.Links {
		ws.Links = append(ws.Links, Link{Src: l.Src, Alias: l.Alias})
	}
	return ws
}

func ToProfile(ws Workspace) config.Profile {
	p := config.Profile{Name: ws.Name}
	for _, l := range ws.Links {
		p.Links = append(p.Links, config.LinkEntry{Src: l.Src, Alias: l.Alias})
	}
	return p
}
