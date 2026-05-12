package execution

// BuildExecutionLevels groups capabilities into dependency levels.
// Level 0 has no dependencies; level N depends only on capabilities in levels < N.
func BuildExecutionLevels(order []string, deps map[string][]string) [][]string {
	if len(order) == 0 {
		return nil
	}

	assigned := make(map[string]int) // capability -> level index
	levels := make([][]string, 0)

	for _, cap := range order {
		level := 0
		if capDeps, ok := deps[cap]; ok {
			for _, d := range capDeps {
				if dl, found := assigned[d]; found && dl+1 > level {
					level = dl + 1
				}
			}
		}
		assigned[cap] = level

		for len(levels) <= level {
			levels = append(levels, nil)
		}
		levels[level] = append(levels[level], cap)
	}

	return levels
}
