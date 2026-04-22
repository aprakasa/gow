package app

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/aprakasa/gow/internal/allocator"
	"github.com/aprakasa/gow/internal/state"
)

func formatSites(w io.Writer, sites []state.Site) error {
	if len(sites) == 0 {
		_, err := fmt.Fprintln(w, "No sites configured.")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "SITE\tTYPE\tPHP\tPRESET\tSTATUS\tBACKUP"); err != nil {
		return err
	}
	for _, s := range sites {
		status := "online"
		if s.Maintenance {
			status = "offline"
		}
		sType := s.Type
		if sType == "" {
			sType = "wp"
		}
		php := s.PHPVersion
		preset := s.Preset
		if sType == "html" {
			php = "-"
			preset = "-"
		}
		backup := "-"
		if s.BackupSchedule != "" {
			backup = s.BackupSchedule
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", s.Name, sType, php, preset, status, backup); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func formatPresets(w io.Writer) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "PRESET\tMEM\tWORKER\tDESCRIPTION"); err != nil {
		return err
	}
	for _, name := range []string{"lite", "standard", "business", "woocommerce", "heavy"} {
		p, _ := allocator.LookupPreset(name)
		if _, err := fmt.Fprintf(tw, "%s\t%dMB\t%dMB\t%s\n", p.Name, p.PHPMemoryLimitMB, p.WorkerBudgetMB, p.Description); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func formatStatus(w io.Writer, totalRAMMB uint64, allocs []allocator.Allocation, p allocator.Policy) error {
	if len(allocs) == 0 {
		_, err := fmt.Fprintln(w, "No sites configured.")
		return err
	}

	budget := allocator.PHPBudget(totalRAMMB, p)
	var budgeted uint64
	for _, a := range allocs {
		budgeted += uint64(a.Children) * a.WorkerBudgetMB //nolint:gosec // Children is bounded by cpuCeiling
	}
	headroom := budget - min(budgeted, budget)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "SITE\tPRESET\tCHILDREN\tHARD LIMIT\tNOTE"); err != nil {
		return err
	}
	for _, a := range allocs {
		note := ""
		if a.Downgraded {
			note = "(downgraded)"
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%d\t%dMB\t%s\n", a.Site, a.PresetUsed, a.Children, a.MemHardMB, note); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(tw, "\nPHP Budget: %dMB\tBudgeted: %dMB\tHeadroom: %dMB\n", budget, budgeted, headroom); err != nil {
		return err
	}
	return tw.Flush()
}
