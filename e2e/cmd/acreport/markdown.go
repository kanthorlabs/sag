package main

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
)

func writeMarkdown(w io.Writer, r report) error {
	var b strings.Builder
	commit := r.Commit
	if r.Dirty {
		commit += " (dirty)"
	}
	gateResult := statusPass
	if !r.Gate.Passed {
		gateResult = statusFail + ": " + strings.Join(r.Gate.Failing, ", ")
	}
	fmt.Fprintf(&b, "# AC report\n\n")
	fmt.Fprintf(&b, "- Commit: `%s`\n", commit)
	fmt.Fprintf(&b, "- Image: `%s`\n", r.Image)
	fmt.Fprintf(&b, "- Image version: `%s`\n", r.ImageVersion)
	fmt.Fprintf(&b, "- Generated: %s\n", r.GeneratedAt)
	fmt.Fprintf(&b, "- Gate layer %d: %s\n\n", r.Gate.Layer, cell(gateResult))

	header := []string{"AC", "Layer", "Test", "Status"}
	for _, major := range r.PGMajors {
		header = append(header, "PG"+strconv.Itoa(major))
	}
	header = append(header, "Duration", "Message")
	writeRow(&b, header)
	separator := make([]string, len(header))
	for i := range separator {
		separator[i] = "---"
	}
	writeRow(&b, separator)

	for _, ac := range r.ACs {
		row := []string{ac.ID, strconv.Itoa(ac.Layer), "`" + ac.Test + "`", ac.Status}
		for _, major := range r.PGMajors {
			row = append(row, ac.ByMajor[strconv.Itoa(major)])
		}
		duration := ""
		if ac.Status != statusNotImplemented && ac.Status != statusExcluded {
			duration = fmt.Sprintf("%d ms", ac.DurationMS)
		}
		row = append(row, duration, cell(ac.Message))
		writeRow(&b, row)
	}

	b.WriteString("\n## Other subtests\n\n")
	otherHeader := []string{"Test", "Status"}
	for _, major := range r.PGMajors {
		otherHeader = append(otherHeader, "PG"+strconv.Itoa(major))
	}
	writeRow(&b, otherHeader)
	writeRow(&b, slices.Repeat([]string{"---"}, len(otherHeader)))
	for _, o := range r.Others {
		row := []string{"`" + o.Test + "`", o.Status}
		for _, major := range r.PGMajors {
			row = append(row, o.ByMajor[strconv.Itoa(major)])
		}
		writeRow(&b, row)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func writeRow(b *strings.Builder, cells []string) {
	b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
}

func cell(s string) string {
	return strings.NewReplacer("|", `\|`, "\r", " ", "\n", " ").Replace(s)
}
