package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

func style(code, text string) string {
	if !colorEnabled() {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func colorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func formatVersion(mtime time.Time) string {
	return mtime.Format("v0601021504")
}

type installedGroup struct {
	Scope    string
	Platform string
	Location string
	Rows     []string
}

func printAbout(name, kind string, fields [][2]string, installs [][3]string, text string) {
	fmt.Println(style("1", name) + "  " + style("2", kind))
	fmt.Println()
	label := 0
	for _, f := range fields {
		if n := utf8.RuneCountInString(f[0]); n > label {
			label = n
		}
	}
	label += 2
	for _, f := range fields {
		fmt.Println("  " + style("2", padRight(f[0], label)) + f[1])
	}
	fmt.Println()
	fmt.Println("  " + style("1;4", "Installed"))
	if len(installs) == 0 {
		fmt.Println("    " + style("2", "not installed"))
	} else {
		for _, scope := range []string{"global", "local"} {
			var rows [][2]string
			for _, inst := range installs {
				if inst[0] == scope {
					rows = append(rows, [2]string{inst[1], inst[2]})
				}
			}
			if len(rows) == 0 {
				continue
			}
			heading := "Global"
			if scope == "local" {
				heading = "Project  " + style("2", display(cwd()))
			}
			fmt.Println("    " + heading)
			for _, row := range rows {
				fmt.Printf("      %s %s\n", row[1], style("36", row[0]))
			}
		}
	}
	if text != "" {
		fmt.Println()
		for _, line := range wrapText(text, 74) {
			fmt.Println("  " + line)
		}
	}
}

func padRight(s string, n int) string {
	for utf8.RuneCountInString(s) < n {
		s += " "
	}
	return s
}

func cwd() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

func wrapText(text string, width int) []string {
	var lines []string
	for _, para := range strings.Split(text, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		line := words[0]
		for _, w := range words[1:] {
			if utf8.RuneCountInString(line)+1+utf8.RuneCountInString(w) > width {
				lines = append(lines, line)
				line = w
			} else {
				line += " " + w
			}
		}
		lines = append(lines, line)
	}
	return lines
}

func renderInstalled(sections []struct {
	Title  string
	Groups []installedGroup
}, scopes []string, project string) {
	width := 0
	for _, p := range platformOrder {
		if len(p) > width {
			width = len(p)
		}
	}
	first := true
	for _, scope := range scopes {
		if !first {
			fmt.Println()
		}
		first = false
		if scope == "global" {
			fmt.Println(style("1", "Global"))
		} else {
			fmt.Println(style("1", "Project") + "  " + style("2", display(project)))
		}
		printed := false
		for _, section := range sections {
			var filled []installedGroup
			for _, g := range section.Groups {
				if g.Scope == scope && len(g.Rows) > 0 {
					filled = append(filled, g)
				}
			}
			if len(filled) == 0 {
				continue
			}
			fmt.Println()
			fmt.Println("  " + style("1;4", section.Title))
			for _, group := range filled {
				fmt.Printf("    %s  %s\n", style("36", padRight(group.Platform, width)), style("2", display(group.Location)))
				for _, row := range group.Rows {
					fmt.Printf("      %s\n", row)
				}
			}
			printed = true
		}
		if !printed {
			fmt.Println("  " + style("2", "nothing installed"))
		}
	}
}

// printPlan renders an install/uninstall plan like the Python version.
func printPlan(heading string, plan *Plan) {
	fmt.Println(heading)
	skillRoot := resolve(plan.Target.SkillDir)
	var inside []string
	var outside []Action
	for _, action := range plan.Actions {
		if action.Kind == "copy" && isInside(resolve(action.Path), skillRoot) {
			rel, _ := filepath.Rel(skillRoot, resolve(action.Path))
			inside = append(inside, filepath.ToSlash(rel))
		} else if action.Kind == "copy" {
			outside = append(outside, action)
		}
	}
	if len(inside) > 0 {
		fmt.Printf("  %s/\n", display(plan.Target.SkillDir))
		for _, rel := range inside {
			fmt.Printf("    %s\n", rel)
		}
	}
	for _, action := range outside {
		fmt.Printf("  copy %s\n", display(action.Path))
	}
	for _, action := range plan.Actions {
		if action.Kind == "remove" {
			fmt.Printf("  remove %s\n", display(action.Path))
		} else if action.Kind == "keep" {
			fmt.Printf("  keep %s (%s)\n", display(action.Path), action.Note)
		}
	}
	fmt.Println()
}
