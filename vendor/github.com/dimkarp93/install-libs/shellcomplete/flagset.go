package shellcomplete

import "flag"

func FromFlagSet(fs *flag.FlagSet) []Flag {
	var out []Flag
	fs.VisitAll(func(f *flag.Flag) {
		v, ok := f.Value.(interface{ IsBoolFlag() bool })
		out = append(out, Flag{Name: "-" + f.Name, Bool: ok && v.IsBoolFlag()})
	})
	return out
}

func With(flags []Flag, overrides ...Flag) []Flag {
	out := make([]Flag, 0, len(flags)+len(overrides))
	for _, f := range flags {
		if o, ok := find(overrides, f.Name); ok {
			f.Values, f.Dirs, f.Files = o.Values, o.Dirs, o.Files
		}
		out = append(out, f)
	}
	for _, o := range overrides {
		if _, ok := find(flags, o.Name); !ok {
			out = append(out, o)
		}
	}
	return out
}
