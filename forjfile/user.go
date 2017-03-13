package forjfile

type UserStruct struct {
	forge *ForgeYaml
	Role string
	More map[string]string `yaml:",inline"`
}

// TODO: Add struct unit tests

func (u *UserStruct) Get(field string) (value string, found bool) {
	switch field {
	case "role":
		return u.Role, (u.Role != "")
	default:
		value, found = u.More[field]
	}
	return
}

func (r *UserStruct)SetHandler(from func(field string)(string, bool), keys...string) {
	for _, key := range keys {
		if v, found := from(key) ; found {
			r.Set(key, v)
		}
	}
}

func (u *UserStruct) Set(field, value string) {
	switch field {
	case "role":
		if u.Role != value {
			u.Role = value
			u.forge.dirty()
		}
	default:
		if u.More == nil {
			u.More = make(map[string]string)
		}
		if v, found := u.More[field] ; found && v != value {
			u.More[field] = value
			u.forge.dirty()
		}
	}
	return
}

func (r *UserStruct)set_forge(f *ForgeYaml) {
	r.forge = f
}
