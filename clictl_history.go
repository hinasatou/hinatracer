package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"

	"hinatracer/internal/config"
	"hinatracer/internal/history"
	"hinatracer/internal/webserver"
)

// ctlHistory prints or exports history for q (id=, or tab=&ttl=).
func (c *ctl) ctlHistory(q url.Values) error {
	rng := c.flags["range"]
	if rng == "" {
		rng = "probe"
	}
	if !history.ValidRange(rng) {
		return errors.New("--range must be probe, minute, hour, day, week or month")
	}
	q.Set("range", rng)
	if _, ok := c.flags["csv"]; ok || c.flags["out"] != "" {
		body, err := c.call("GET", "/api/history.csv?"+q.Encode(), nil)
		if err != nil {
			return err
		}
		if p := c.flags["out"]; p != "" && p != "-" {
			if err := os.WriteFile(p, body, 0o644); err != nil {
				return err
			}
			fmt.Fprintln(c.out, "saved", p)
			return nil
		}
		_, err = c.out.Write(body)
		return err
	}
	if v := c.flags["limit"]; v != "" {
		q.Set("limit", v)
	}
	body, err := c.call("GET", "/api/history?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	var r struct {
		Data struct {
			Key    string          `json:"key"`
			Range  string          `json:"range"`
			Points []history.Point `json:"points"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return err
	}
	if c.json {
		c.printJSON(r.Data)
		return nil
	}
	rows := make([][]string, 0, len(r.Data.Points))
	for _, p := range r.Data.Points {
		rows = append(rows, []string{p.At.Local().Format("2006-01-02 15:04:05"), strconv.Itoa(p.Samples),
			ms(p.Avg, p.OK), ms(p.Min, p.OK), ms(p.Max, p.OK), fmt.Sprintf("%.1f%%", p.LossPct)})
	}
	c.table([]string{"TIME", "SAMPLES", "AVG", "MIN", "MAX", "LOSS"}, rows)
	return nil
}

// runWebPasswd sets or clears the WebUI password directly in the config
// file (no server needed). The password is read from stdin (--stdin) or
// prompted without echo. Never printed or logged.
func runWebPasswd(flags map[string]string, in io.Reader, out, errOut io.Writer) int {
	if p := flags["config"]; p != "" {
		config.OverridePath = p
	}
	path := config.ResolvePath()
	st := config.NewStore(path)
	if err := st.Load(); err != nil {
		fmt.Fprintln(errOut, "error: cannot read config:", err)
		return 1
	}
	if flags["clear"] == "true" {
		if err := st.Update(func(c *config.Config) { c.WebPasswordHash = ""; c.WebGuest = false }); err != nil {
			fmt.Fprintln(errOut, "error:", err)
			return 1
		}
		fmt.Fprintln(out, "password cleared in", path)
		return 0
	}
	var pw string
	if flags["stdin"] == "true" {
		line, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && line == "" {
			fmt.Fprintln(errOut, "error: no password on stdin")
			return 1
		}
		pw = strings.TrimRight(line, "\r\n")
	} else {
		fd := int(os.Stdin.Fd())
		if !term.IsTerminal(fd) {
			fmt.Fprintln(errOut, "error: not a terminal; use --stdin")
			return 2
		}
		fmt.Fprint(out, "New password: ")
		a, err := term.ReadPassword(fd)
		fmt.Fprintln(out)
		if err != nil {
			fmt.Fprintln(errOut, "error:", err)
			return 1
		}
		fmt.Fprint(out, "Repeat: ")
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(out)
		if err != nil {
			fmt.Fprintln(errOut, "error:", err)
			return 1
		}
		if string(a) != string(b) {
			fmt.Fprintln(errOut, "error: passwords do not match")
			return 1
		}
		pw = string(a)
	}
	if len([]rune(pw)) < 6 {
		fmt.Fprintln(errOut, "error: password must be at least 6 characters")
		return 1
	}
	h, err := webserver.HashPassword(pw)
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return 1
	}
	if err := st.Update(func(c *config.Config) { c.WebPasswordHash = h }); err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return 1
	}
	fmt.Fprintln(out, "password saved in", path, "(restart the server to apply)")
	return 0
}
