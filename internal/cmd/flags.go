package cmd

import "strings"

func normalizeArgs(args []string, flagsWithValues map[string]bool) ([]string, []string) {
	var normalized []string
	var positional []string

	for index := 0; index < len(args); index++ {
		arg := args[index]
		if strings.HasPrefix(arg, "-") {
			normalized = append(normalized, arg)
			if flagsWithValues[arg] && index+1 < len(args) {
				index++
				normalized = append(normalized, args[index])
			}
			continue
		}
		positional = append(positional, arg)
	}

	return normalized, positional
}
