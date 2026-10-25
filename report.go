package main

import (
	"fmt"
	"io"
	"strings"
)

func printReport(w io.Writer, r Report) {
	fmt.Fprintf(w, "denoexit: %s (deno vs %s)\n\n", r.App, r.SecondTarget)
	fmt.Fprintf(w, "Deno APIs found in source:\n")
	if len(r.APIs) == 0 {
		fmt.Fprintf(w, "  none\n")
	}
	for _, a := range r.APIs {
		tag := "MISSING"
		if a.Shimmed {
			tag = "shimmed"
		}
		fmt.Fprintf(w, "  %-8s %-22s %-14s %s\n", tag, a.API, a.Where, a.Hint)
	}
	fmt.Fprintf(w, "\nBoot:\n")
	fmt.Fprintf(w, "  deno     %s\n", bootLine(r.DenoBoot))
	fmt.Fprintf(w, "  workerd  %s\n", bootLine(r.WorkerdBoot))
	fmt.Fprintf(w, "\nReplay:\n")
	same := 0
	for _, x := range r.Results {
		v := "DIFF"
		if x.Same {
			v = "SAME"
			same++
		}
		fmt.Fprintf(w, "  %-4s %-28s deno %s | workerd %s\n", v, x.Name, replyLine(x.Deno), replyLine(x.Workerd))
		if !x.Same && x.Workerd.Error != "" {
			fmt.Fprintf(w, "       workerd error: %s\n", x.Workerd.Error)
		}
	}
	verdict := "READY"
	if !r.OK {
		verdict = "NOT READY"
	}
	fmt.Fprintf(w, "\n%s: %d/%d requests match, workerd boot ok=%v\n", verdict, same, len(r.Results), r.WorkerdBoot.OK)
}

func bootLine(b Boot) string {
	if b.OK {
		return "ok"
	}
	return "FAILED: " + b.Error
}

func replyLine(r Reply) string {
	if r.Status == 0 {
		return "-"
	}
	body := strings.ReplaceAll(r.Body, "\n", `\n`)
	if len(body) > 40 {
		body = body[:37] + "..."
	}
	return fmt.Sprintf("%d %s", r.Status, body)
}
