package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func splitSchema(s string) [2]string {
	parts := strings.SplitN(s, ".", 2)
	if len(parts) == 2 {
		return [2]string{parts[0], parts[1]}
	}
	return [2]string{"public", s}
}

func main() {
	_ = godotenv.Load("../../.env")
	_ = godotenv.Load("../.env")

	dbURL := os.Getenv("DATABASE_URL")
	sessionID := "bbdb19b0-be52-47e3-bcd4-fffc50419bc9"
	if len(os.Args) > 1 {
		sessionID = os.Args[1]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	// 0. List all tables
	fmt.Println("=== TABLES ===")
	rows0, _ := pool.Query(ctx, `
		SELECT table_schema, table_name FROM information_schema.tables
		WHERE table_schema NOT IN ('pg_catalog', 'information_schema')
		ORDER BY table_schema, table_name
	`)
	if rows0 != nil {
		defer rows0.Close()
		for rows0.Next() {
			var schema, name string
			rows0.Scan(&schema, &name)
			fmt.Printf("  %s.%s\n", schema, name)
		}
	}

	// 0b. Show column info for key tables
	for _, tbl := range []string{"public.pipeline_traces", "public.chat_session_state", "public.chat_session_deltas"} {
		fmt.Printf("\n--- Columns of %s ---\n", tbl)
		parts := splitSchema(tbl)
		colRows, _ := pool.Query(ctx, `
			SELECT column_name, data_type FROM information_schema.columns
			WHERE table_schema=$1 AND table_name=$2 ORDER BY ordinal_position
		`, parts[0], parts[1])
		if colRows != nil {
			for colRows.Next() {
				var cn, dt string
				colRows.Scan(&cn, &dt)
				fmt.Printf("  %s (%s)\n", cn, dt)
			}
			colRows.Close()
		}
	}

	// 1. Pipeline traces
	fmt.Println("\n=== PIPELINE TRACES ===")
	for _, table := range []string{"public.pipeline_traces"} {
		rows, err := pool.Query(ctx, fmt.Sprintf(`
			SELECT COALESCE(query, ''), COALESCE(error, ''), total_ms,
			       COALESCE(trace_data::text, '{}')
			FROM %s
			WHERE session_id = $1
			ORDER BY timestamp
		`, table), sessionID)
		if err != nil {
			continue
		}
		fmt.Printf("Found in %s:\n", table)
		defer rows.Close()
		for rows.Next() {
			var query, errMsg, traceData string
			var totalMs int
			rows.Scan(&query, &errMsg, &totalMs, &traceData)
			fmt.Printf("query=%q | err=%q | %dms\n", query, errMsg, totalMs)
			if len(traceData) > 2 && len(traceData) < 2000 {
				fmt.Printf("  trace: %s\n", traceData)
			} else if len(traceData) >= 2000 {
				fmt.Printf("  trace (first 2000): %s...\n", traceData[:2000])
			}
		}
		break
	}

	// 2. State
	fmt.Println("\n=== STATE ===")
	for _, table := range []string{"public.chat_session_state"} {
		var currentJSON, metaJSON string
		var step int
		err = pool.QueryRow(ctx, fmt.Sprintf(`
			SELECT step, COALESCE(LEFT(current_data::text, 500), '{}'),
			       COALESCE(current_meta::text, '{}')
			FROM %s
			WHERE session_id::text = $1
		`, table), sessionID).Scan(&step, &currentJSON, &metaJSON)
		if err != nil {
			continue
		}
		fmt.Printf("Found in %s:\n", table)
		fmt.Printf("Step: %d\nData (first 500): %s\nMeta: %s\n", step, currentJSON, metaJSON)
		break
	}

	// 3. Deltas
	fmt.Println("\n=== DELTAS ===")
	for _, table := range []string{"public.chat_session_deltas"} {
		rows2, err := pool.Query(ctx, fmt.Sprintf(`
			SELECT step, COALESCE(path, ''), COALESCE(action::text, '{}'), COALESCE(result::text, '{}')
			FROM %s
			WHERE session_id::text = $1
			ORDER BY step
		`, table), sessionID)
		if err != nil {
			continue
		}
		fmt.Printf("Found in %s:\n", table)
		defer rows2.Close()
		for rows2.Next() {
			var dStep int
			var path, action, result string
			rows2.Scan(&dStep, &path, &action, &result)
			fmt.Printf("Delta step=%d | path=%s | action=%s | result=%s\n", dStep, path, action, result)
		}
		break
	}

	// 3b. Raw query on pipeline_traces for this session
	fmt.Println("\n=== RAW PIPELINE TRACES ===")
	rawRows, err := pool.Query(ctx, `SELECT * FROM public.pipeline_traces WHERE session_id = $1 ORDER BY timestamp`, sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "raw traces: %v\n", err)
	} else {
		cols := rawRows.FieldDescriptions()
		colNames := make([]string, len(cols))
		for i, c := range cols {
			colNames[i] = string(c.Name)
		}
		fmt.Printf("Columns: %v\n", colNames)
		count := 0
		for rawRows.Next() {
			vals, _ := rawRows.Values()
			fmt.Printf("Row %d: ", count)
			for i, v := range vals {
				if i < len(colNames) {
					s := fmt.Sprintf("%v", v)
					if len(s) > 200 {
						s = s[:200] + "..."
					}
					fmt.Printf("%s=%s | ", colNames[i], s)
				}
			}
			fmt.Println()
			count++
		}
		rawRows.Close()
		if count == 0 {
			fmt.Println("  (no rows)")
		}
	}

	// 4. Check recent traces (any session)
	fmt.Println("\n=== RECENT TRACES (any session, last 5) ===")
	recentRows, err := pool.Query(ctx, `
		SELECT session_id, COALESCE(query, ''), COALESCE(error, ''), total_ms, timestamp
		FROM public.pipeline_traces
		ORDER BY timestamp DESC
		LIMIT 5
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "recent traces: %v\n", err)
	} else {
		defer recentRows.Close()
		for recentRows.Next() {
			var sid, q, e string
			var ms int
			var ts time.Time
			recentRows.Scan(&sid, &q, &e, &ms, &ts)
			fmt.Printf("  %s | session=%s | query=%q | err=%q | %dms\n", ts.Format("15:04:05"), sid, q, e, ms)
		}
	}

	// 4b. Check recent sessions
	fmt.Println("\n=== RECENT SESSIONS (last 5) ===")
	sessRows, err := pool.Query(ctx, `
		SELECT id, status, started_at, last_activity_at
		FROM public.chat_sessions
		ORDER BY last_activity_at DESC
		LIMIT 5
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sessions: %v\n", err)
	} else {
		defer sessRows.Close()
		for sessRows.Next() {
			var id, status string
			var started, lastAct time.Time
			sessRows.Scan(&id, &status, &started, &lastAct)
			fmt.Printf("  %s | status=%s | started=%s | last=%s\n", id, status, started.Format("15:04:05"), lastAct.Format("15:04:05"))
		}
	}

	// 4c. Trace data for latest successful trace
	fmt.Println("\n=== LATEST TRACE DATA ===")
	var traceJSON string
	var traceSessionID string
	err = pool.QueryRow(ctx, `
		SELECT session_id, COALESCE(trace_data::text, '{}')
		FROM public.pipeline_traces
		WHERE error = '' OR error IS NULL
		ORDER BY timestamp DESC LIMIT 1
	`).Scan(&traceSessionID, &traceJSON)
	if err != nil {
		fmt.Fprintf(os.Stderr, "trace data: %v\n", err)
	} else {
		fmt.Printf("Session: %s\n", traceSessionID)
		if len(traceJSON) > 3000 {
			fmt.Printf("Trace (first 3000): %s...\n", traceJSON[:3000])
		} else {
			fmt.Printf("Trace: %s\n", traceJSON)
		}
	}

	// 4d. State for this session
	fmt.Println("\n=== STATE FOR LATEST SESSION ===")
	var stateDataJSON, stateMetaJSON string
	var stateStep int
	err = pool.QueryRow(ctx, `
		SELECT step, COALESCE(LEFT(current_data::text, 1000), '{}'),
		       COALESCE(current_meta::text, '{}')
		FROM public.chat_session_state
		WHERE session_id::text = $1
	`, traceSessionID).Scan(&stateStep, &stateDataJSON, &stateMetaJSON)
	if err != nil {
		fmt.Fprintf(os.Stderr, "state: %v\n", err)
	} else {
		fmt.Printf("Step: %d\nMeta: %s\nData (first 1000): %s\n", stateStep, stateMetaJSON, stateDataJSON)
	}

	// 5. Check embeddings
	fmt.Println("\n=== EMBEDDINGS STATUS ===")
	var totalMP, withEmb, withoutEmb int
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM catalog.master_products`).Scan(&totalMP)
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM catalog.master_products WHERE embedding IS NOT NULL`).Scan(&withEmb)
	withoutEmb = totalMP - withEmb
	fmt.Printf("Master products: %d total, %d with embedding, %d without\n", totalMP, withEmb, withoutEmb)
}
