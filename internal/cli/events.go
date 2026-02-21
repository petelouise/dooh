package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"dooh/internal/db"
	"dooh/internal/idgen"
)

func writeEvent(sqlite db.SQLite, p principal, eventType string, aggregateType string, aggregateID string, payload any) error {
	if strings.TrimSpace(p.UserID) == "" {
		return errors.New("missing actor user for event")
	}
	eventID, err := idgen.ULIDLike()
	if err != nil {
		return err
	}
	outboxID, err := idgen.ULIDLike()
	if err != nil {
		return err
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	payloadStr := string(b)
	topic := "website." + eventType
	sql := fmt.Sprintf("BEGIN; INSERT INTO events(id,event_type,aggregate_type,aggregate_id,actor_user_id,key_id,client_type,payload_json) VALUES(%s,%s,%s,%s,%s,%s,%s,%s); INSERT INTO outbox(id,event_id,topic,payload_json,status,available_at) VALUES(%s,%s,%s,%s,'pending',strftime('%%Y-%%m-%%dT%%H:%%M:%%fZ','now')); COMMIT;",
		db.Quote(eventID), db.Quote(eventType), db.Quote(aggregateType), db.Quote(aggregateID), db.Quote(p.UserID), db.Quote(p.KeyID), db.Quote(p.ClientType), db.Quote(payloadStr),
		db.Quote(outboxID), db.Quote(eventID), db.Quote(topic), db.Quote(payloadStr),
	)
	return sqlite.Exec(sql)
}

func runEvent(rt runtime, args []string, out io.Writer) error {
	if len(args) == 0 {
		return printEventHelp(out)
	}
	switch args[0] {
	case "list":
		return runEventList(rt, args[1:], out)
	case "help", "--help", "-h":
		return printEventHelp(out)
	default:
		return fmt.Errorf("unknown event command %q (available: list)", args[0])
	}
}

func printEventHelp(out io.Writer) error {
	_, _ = fmt.Fprintln(out, "event subcommands:")
	_, _ = fmt.Fprintln(out, "  list   query audit event log")
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "flags for event list:")
	_, _ = fmt.Fprintln(out, "  --limit N              max events to return (default 20)")
	_, _ = fmt.Fprintln(out, "  --type <event_type>    filter by event type (e.g. task.created)")
	_, _ = fmt.Fprintln(out, "  --actor <user_id>      filter by actor user ID")
	_, _ = fmt.Fprintln(out, "  --client-type <type>   filter by client type (human_cli|agent_cli)")
	_, _ = fmt.Fprintln(out, "  --since <timestamp>    events after this ISO8601 timestamp")
	return nil
}

func runEventList(rt runtime, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("event list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	limit := fs.Int("limit", 20, "max events to return")
	eventType := fs.String("type", "", "filter by event type")
	actor := fs.String("actor", "", "filter by actor user ID")
	clientType := fs.String("client-type", "", "filter by client type")
	since := fs.String("since", "", "events after this timestamp")
	if err := fs.Parse(args); err != nil {
		return err
	}

	sqlite := db.New(resolveDB(rt, *dbPath))
	if _, err := mustReadAuth(rt, sqlite, *apiKey, "tasks:read"); err != nil {
		return err
	}

	where := []string{}
	if strings.TrimSpace(*eventType) != "" {
		where = append(where, fmt.Sprintf("event_type=%s", db.Quote(*eventType)))
	}
	if strings.TrimSpace(*actor) != "" {
		where = append(where, fmt.Sprintf("actor_user_id=%s", db.Quote(*actor)))
	}
	if strings.TrimSpace(*clientType) != "" {
		where = append(where, fmt.Sprintf("client_type=%s", db.Quote(*clientType)))
	}
	if strings.TrimSpace(*since) != "" {
		where = append(where, fmt.Sprintf("occurred_at>%s", db.Quote(*since)))
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	query := fmt.Sprintf("SELECT seq,event_type,aggregate_type,aggregate_id,actor_user_id,client_type,occurred_at,payload_json FROM events %s ORDER BY seq DESC LIMIT %d;", whereClause, *limit)
	rows, err := sqlite.QueryTSV(query)
	if err != nil {
		return err
	}

	if rt.opts.JSON {
		events := make([]map[string]any, 0, len(rows))
		for _, r := range rows {
			if len(r) >= 8 {
				e := map[string]any{
					"seq":            parseIntDefault(r[0], 0),
					"event_type":     r[1],
					"aggregate_type": r[2],
					"aggregate_id":   r[3],
					"actor_user_id":  r[4],
					"client_type":    r[5],
					"occurred_at":    r[6],
				}
				var payload any
				if json.Unmarshal([]byte(r[7]), &payload) == nil {
					e["payload"] = payload
				} else {
					e["payload"] = r[7]
				}
				events = append(events, e)
			}
		}
		return writeJSON(out, events)
	}

	_, _ = fmt.Fprintln(out, style("SEQ  EVENT_TYPE              ACTOR            CLIENT_TYPE  OCCURRED_AT", "1"))
	_, _ = fmt.Fprintln(out, strings.Repeat("-", 90))
	for _, r := range rows {
		if len(r) >= 7 {
			_, _ = fmt.Fprintf(out, "%-4s %-23s %-16s %-12s %s\n",
				r[0],
				truncate(r[1], 23),
				truncate(r[4], 16),
				r[5],
				truncate(r[6], 24),
			)
		}
	}
	return nil
}
