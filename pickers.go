package main

import "ofam.dev/ochazuke"

// declarePickers declares the tool's noun pickers (ochazuke.PickerDef): the
// catalog lists them and a host mounts them under the tool's prefix. Rows come
// from each noun's list verb and a pick runs the noun's own open or get, as
// the derived pickers do; declaring them gives each an About line.
func declarePickers(r *ochazuke.Runtime) error {
	for _, d := range []ochazuke.PickerDef{
		{Name: "question", Noun: "question", About: "The recorded questions; pick one to open its thread"},
		{Name: "review", Noun: "review", About: "The questions waiting for the owner's decision; pick one to open it"},
	} {
		if err := r.AddPicker(d); err != nil {
			return err
		}
	}
	return nil
}
