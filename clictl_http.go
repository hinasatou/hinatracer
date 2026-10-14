package main

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"

	"hinatracer/internal/httpmon"
)

// httpIDs resolves id, URL or alias references to HTTP target ids.
func (c *ctl) httpIDs(st wsState, refs []string) ([]int, error) {
	var ids []int
	for _, ref := range refs {
		found := false
		if n, err := strconv.Atoi(ref); err == nil {
			for _, r := range st.HTTP.Rows {
				if r.ID == n {
					ids, found = append(ids, n), true
					break
				}
			}
		}
		if !found {
			norm := httpmon.NormalizeURL(ref)
			for _, r := range st.HTTP.Rows {
				if r.Spec.URL == ref || r.Spec.URL == norm || (r.Alias != "" && strings.EqualFold(r.Alias, ref)) {
					ids, found = append(ids, r.ID), true
					break
				}
			}
		}
		if !found {
			return nil, fmt.Errorf("no HTTP target %q", ref)
		}
	}
	return ids, nil
}

// httpSpecFlags applies --method/--interval/... onto s.
func (c *ctl) httpSpecFlags(s *httpmon.Spec) error {
	f := c.flags
	if v, ok := f["method"]; ok {
		s.Method = strings.ToUpper(v)
	}
	if v, ok := f["body"]; ok {
		s.Body = v
	}
	if v, ok := f["content-type"]; ok {
		s.ContentType = v
	}
	for _, k := range []string{"interval", "timeout"} {
		if v, ok := f[k]; ok {
			n, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return fmt.Errorf("--%s: %v", k, err)
			}
			if k == "interval" {
				s.Interval = n
			} else {
				s.Timeout = n
			}
		}
	}
	if v, ok := f["expect"]; ok {
		s.Expect = v
	}
	if v, ok := f["match"]; ok {
		s.Match = v
	}
	if v, ok := f["header"]; ok {
		s.Headers = strings.ReplaceAll(v, `\n`, "\n")
	}
	bf := func(k string, dst *bool) {
		if v, ok := f[k]; ok {
			*dst = v == "true" || v == "1" || v == "yes"
		}
	}
	bf("regex", &s.MatchRegex)
	bf("invert", &s.MatchInvert)
	bf("no-follow", &s.NoFollow)
	bf("insecure", &s.InsecureSkipTLS)
	return nil
}

func (c *ctl) cmdHTTP() error {
	sub := c.sub()
	switch sub {
	case "", "list", "ls":
		st, err := c.state()
		if err != nil {
			return err
		}
		failing := c.flags["failing"] == "true"
		var list []wsHTTP
		var rows [][]string
		for _, r := range st.HTTP.Rows {
			if failing && !r.Failing {
				continue
			}
			r.Hist = nil
			list = append(list, r)
			state := "on"
			switch {
			case !r.Enabled:
				state = "off"
			case r.Checks == 0:
				state = "wait"
			case !r.OK:
				state = "DOWN"
			}
			code := "—"
			if r.Code > 0 {
				code = strconv.Itoa(r.Code)
			}
			ok := r.Success > 0
			rows = append(rows, []string{strconv.Itoa(r.ID), r.Spec.URL, dash(r.Alias), state, code,
				ms(r.Last, r.Checks > 0 && r.OK), ms(r.Avg, ok), ms(r.Median, ok), ms(r.Min, ok), ms(r.Max, ok),
				fmt.Sprintf("%.1f%%", r.SuccessRate), fmt.Sprintf("%.1f%%", r.LossRate), strconv.Itoa(r.Failure),
				dash(r.IP), dash(r.Loc), dash(r.Region), dash(r.ASN), dash(r.ASNRegion)})
		}
		if c.json {
			c.printJSON(map[string]any{"running": st.HTTP.Running, "targets": list})
			return nil
		}
		fmt.Fprintln(c.out, st.HTTP.Status)
		c.table([]string{"ID", "URL", "ALIAS", "STATE", "CODE", "LAST", "AVG", "MEDIAN", "MIN", "MAX", "OK%", "LOSS%", "FAIL", "IP", "LOCATION", "REGION", "ASN", "ASN REGION"}, rows)
		return nil
	case "show":
		if err := c.need(1, "http show <id|url|alias>"); err != nil {
			return err
		}
		st, err := c.state()
		if err != nil {
			return err
		}
		ids, err := c.httpIDs(st, c.args[:1])
		if err != nil {
			return err
		}
		for _, r := range st.HTTP.Rows {
			if r.ID != ids[0] {
				continue
			}
			r.Hist = nil
			if c.json {
				c.printJSON(r)
				return nil
			}
			s := r.Spec.Defaults()
			ok := r.Success > 0
			hdr := "—"
			if s.Headers != "" {
				hdr = strings.ReplaceAll(httpmon.RedactHeaders(s.Headers), "\n", "; ")
			}
			c.table([]string{"FIELD", "VALUE"}, [][]string{
				{"id", strconv.Itoa(r.ID)}, {"url", s.URL}, {"alias", dash(r.Alias)}, {"enabled", strconv.FormatBool(r.Enabled)},
				{"method", s.Method}, {"content type", dash(s.ContentType)}, {"interval s", fmt.Sprint(s.Interval)}, {"timeout s", fmt.Sprint(s.Timeout)},
				{"expect", dash(s.Expect)}, {"match", dash(s.Match)}, {"regex", strconv.FormatBool(s.MatchRegex)},
				{"invert", strconv.FormatBool(s.MatchInvert)}, {"headers", hdr},
				{"follow redirects", strconv.FormatBool(!s.NoFollow)}, {"ignore TLS errors", strconv.FormatBool(s.InsecureSkipTLS)},
				{"code", strconv.Itoa(r.Code)}, {"ok", strconv.FormatBool(r.OK)}, {"error", dash(r.Err)},
				{"last ms", ms(r.Last, r.Checks > 0 && r.OK)}, {"avg ms", ms(r.Avg, ok)}, {"median ms", ms(r.Median, ok)}, {"min ms", ms(r.Min, ok)}, {"max ms", ms(r.Max, ok)},
				{"dns/connect/tls/ttfb ms", fmt.Sprintf("%.1f / %.1f / %.1f / %.1f", r.DNSMs, r.ConnMs, r.TLSMs, r.TTFBMs)},
				{"success", strconv.Itoa(r.Success)}, {"failure", strconv.Itoa(r.Failure)},
				{"success rate", fmt.Sprintf("%.1f%%", r.SuccessRate)}, {"failure rate", fmt.Sprintf("%.1f%%", r.LossRate)},
				{"last ok", fmtCheckTime(r.LastOK)}, {"last failure", fmtCheckTime(r.LastFail)},
				{"cert days", certDaysText(r)}, {"ip", dash(r.IP)}, {"location", dash(r.Loc)}, {"region", dash(r.Region)},
				{"asn", dash(r.ASN)}, {"asn region", dash(r.ASNRegion)}, {"last check", fmtCheckTime(r.LastCheck)},
			})
			return nil
		}
		return errors.New("not found")
	case "add":
		if err := c.need(1, "http add <url> [--alias A] [--method M] [--interval S] ..."); err != nil {
			return err
		}
		spec := httpmon.Spec{URL: c.args[0]}
		if err := c.httpSpecFlags(&spec); err != nil {
			return err
		}
		return c.post("/api/http/add", map[string]any{"alias": c.flags["alias"], "spec": spec})
	case "import":
		if err := c.need(1, "http import <file|->"); err != nil {
			return err
		}
		var data []byte
		var err error
		if c.args[0] == "-" {
			data, err = io.ReadAll(os.Stdin)
		} else {
			data, err = os.ReadFile(c.args[0])
		}
		if err != nil {
			return err
		}
		return c.post("/api/http/addmany", map[string]any{"text": string(data)})
	case "edit":
		if err := c.need(1, "http edit <id|url|alias> [--url U] [--alias A] [--method M] ..."); err != nil {
			return err
		}
		st, err := c.state()
		if err != nil {
			return err
		}
		ids, err := c.httpIDs(st, c.args[:1])
		if err != nil {
			return err
		}
		for _, r := range st.HTTP.Rows {
			if r.ID != ids[0] {
				continue
			}
			spec, alias := r.Spec, r.Alias
			if v, ok := c.flags["url"]; ok {
				spec.URL = v
			}
			if v, ok := c.flags["alias"]; ok {
				alias = v
			}
			if err := c.httpSpecFlags(&spec); err != nil {
				return err
			}
			return c.post("/api/http/update", map[string]any{"id": r.ID, "alias": alias, "spec": spec})
		}
		return errors.New("not found")
	case "delete", "rm", "enable", "disable", "reset":
		if sub != "reset" {
			if err := c.need(1, "http "+sub+" <id|url|alias>..."); err != nil {
				return err
			}
		}
		var ids []int
		if len(c.args) > 0 {
			st, err := c.state()
			if err != nil {
				return err
			}
			if ids, err = c.httpIDs(st, c.args); err != nil {
				return err
			}
		}
		switch sub {
		case "delete", "rm":
			return c.post("/api/http/delete", map[string]any{"ids": ids})
		case "reset":
			return c.post("/api/http/reset", map[string]any{"ids": ids, "confirm": c.yes()})
		}
		return c.post("/api/http/enable", map[string]any{"ids": ids, "enabled": sub == "enable"})
	case "alias":
		if err := c.need(2, "http alias <id|url|alias> <alias>"); err != nil {
			return err
		}
		st, err := c.state()
		if err != nil {
			return err
		}
		ids, err := c.httpIDs(st, c.args[:1])
		if err != nil {
			return err
		}
		return c.post("/api/http/alias", map[string]any{"id": ids[0], "alias": c.args[1]})
	case "move":
		if err := c.need(2, "http move <id|url|alias> up|down"); err != nil {
			return err
		}
		st, err := c.state()
		if err != nil {
			return err
		}
		ids, err := c.httpIDs(st, c.args[:1])
		if err != nil {
			return err
		}
		return c.post("/api/http/move", map[string]any{"id": ids[0], "up": c.args[1] == "up"})
	case "start", "stop":
		return c.post("/api/http/"+sub, nil)
	case "history":
		if err := c.need(1, "http history <id|url|alias> [--range R] [--limit N] [--csv] [--out FILE]"); err != nil {
			return err
		}
		st, err := c.state()
		if err != nil {
			return err
		}
		ids, err := c.httpIDs(st, c.args[:1])
		if err != nil {
			return err
		}
		return c.ctlHistory(url.Values{"http": {strconv.Itoa(ids[0])}})
	}
	return fmt.Errorf("unknown http command %q", sub)
}

func certDaysText(r wsHTTP) string {
	if r.CertDays < 0 || r.Checks == 0 || !strings.HasPrefix(strings.ToLower(r.Spec.URL), "https") {
		return "—"
	}
	return strconv.Itoa(r.CertDays)
}
