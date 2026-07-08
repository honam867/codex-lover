package service

import (
	"sort"
	"strings"
	"time"

	"codex-lover/internal/model"
)

type triggerTarget struct {
	Status          model.ProfileStatus
	SourceProfileID string
}

type triggerSkip struct {
	Status model.ProfileStatus
	Reason string
}

// selectTriggerTargets picks which Codex accounts to trigger. cachedSource
// resolves the profile ID whose cached auth backs a profile (and whether one
// exists). Only enabled Codex accounts with cached auth are eligible.
func selectTriggerTargets(
	statuses []model.ProfileStatus,
	cfg model.TriggerConfig,
	cachedSource func(model.Profile) (string, bool),
) (selected []triggerTarget, skipped []triggerSkip) {
	wanted := map[string]bool{}
	for _, id := range cfg.ProfileIDs {
		wanted[id] = true
	}

	var eligible []triggerTarget
	for _, st := range statuses {
		if st.Profile.Tool != model.ToolCodex || !st.Profile.Enabled {
			continue
		}
		src, ok := cachedSource(st.Profile)
		if !ok {
			if cfg.Mode != model.TriggerModeCustom || wanted[st.Profile.ID] {
				skipped = append(skipped, triggerSkip{Status: st, Reason: model.TriggerStatusSkippedNoAuth})
			}
			continue
		}
		eligible = append(eligible, triggerTarget{Status: st, SourceProfileID: src})
	}

	switch cfg.Mode {
	case model.TriggerModeCustom:
		for _, t := range eligible {
			if wanted[t.Status.Profile.ID] {
				selected = append(selected, t)
			}
		}
	case model.TriggerModeTopN:
		sort.SliceStable(eligible, func(i, j int) bool {
			wi, wj := weeklyRemaining(eligible[i].Status), weeklyRemaining(eligible[j].Status)
			if wi != wj {
				return wi > wj
			}
			pi, pj := fiveHourRemaining(eligible[i].Status), fiveHourRemaining(eligible[j].Status)
			if pi != pj {
				return pi > pj
			}
			return strings.ToLower(profileLabel(eligible[i].Status.Profile)) <
				strings.ToLower(profileLabel(eligible[j].Status.Profile))
		})
		n := cfg.Count
		if n < 1 {
			n = 1
		}
		if n > len(eligible) {
			n = len(eligible)
		}
		selected = append(selected, eligible[:n]...)
	default: // all
		selected = eligible
	}
	return selected, skipped
}

func shouldFireTrigger(now time.Time, cfg model.TriggerConfig, lastDate string) (bool, string) {
	if !cfg.Enabled {
		return false, "disabled"
	}
	scheduled, err := parseTimeOfDay(now, cfg.TimeOfDay)
	if err != nil {
		return false, "invalid time_of_day"
	}
	if now.Format("2006-01-02") == lastDate {
		return false, "already ran today"
	}
	if now.Before(scheduled) {
		return false, "before scheduled time"
	}
	grace := cfg.GraceMins
	if grace <= 0 {
		grace = 60
	}
	if now.After(scheduled.Add(time.Duration(grace) * time.Minute)) {
		return false, "missed grace window"
	}
	return true, "in window"
}

func parseTimeOfDay(now time.Time, hhmm string) (time.Time, error) {
	t, err := time.ParseInLocation("15:04", strings.TrimSpace(hhmm), now.Location())
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location()), nil
}
