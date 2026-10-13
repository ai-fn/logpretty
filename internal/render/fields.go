package render

import "slices"

// role is what a name in a selection means besides a field of that name.
type role uint8

const (
	roleNone role = iota
	roleMsg
	roleTime
	roleLevel
	roleCount // the length of an array indexed by a role
)

// Selection is a choice of what to show of an entry besides its prefix, its time and its level.
type Selection struct {
	fields           []selected // each name once, in the order asked for
	msg, time, level bool       // whether the name of that part is among them
}

type selected struct {
	name []byte
	role role
}

// Select returns the choice of the message, if "msg" is among the names, and of the fields named, in
// that order. A name is a path, as in "http.status". Without names it returns nil: every field is shown.
func Select(names []string) *Selection {
	if len(names) == 0 {
		return nil
	}
	s := &Selection{}
	for i, name := range names {
		if slices.Contains(names[:i], name) {
			continue
		}
		field := selected{name: []byte(name)}
		switch name {
		case "msg":
			field.role, s.msg = roleMsg, true
		case "time":
			field.role, s.time = roleTime, true
		case "level":
			field.role, s.level = roleLevel, true
		}
		s.fields = append(s.fields, field)
	}
	return s
}
