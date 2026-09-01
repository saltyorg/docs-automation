package automation

// Index reports the current index-generation implementation status.
func (r *Runner) Index() error {
	r.printf("Index generation is not yet implemented.\n\n")
	r.printf("This command will eventually:\n")
	for _, line := range []string{
		"  1. Scan all app documentation files",
		"  2. Read categories from saltbox_automation.project_description.categories",
		"  3. Generate categorized index.md files",
	} {
		r.printf("%s\n", line)
	}
	return r.result(nil)
}
