// Package legacycore maps the reviewed staging snapshot for a local rehearsal.
package legacycore

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"lmsv3/internal/migration/foundation"
)

const Snapshot = "20261118T131839Z-staging-baseline-3fe3bf70b4d4"
const SnapshotHash = "fd35763e23e0d300b15288383076ba78e116d48fd7b14a609acba0ec453a4eca"
const Mapping = "legacy-core-v1"
const originNamespace = "lms-v3-local:legacy-staging-public"
const ObservedAt = "2026-11-18T13:18:40.455280Z"
const SnapshotSize = 2121156

var ErrSource = errors.New("CORE_SNAPSHOT_REJECTED")
var ErrMapping = errors.New("CORE_MAPPING_REJECTED")
var copyHeader = regexp.MustCompile(`^COPY public\.([a-z_]+) \(([^)]+)\) FROM stdin;$`)
var columnPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

var projections = map[string][]string{
	"programs":    {"id", "name"},
	"batches":     {"id", "name"},
	"classes":     {"id", "programId", "batchId", "mentorId"},
	"users":       {"id", "status", "roles"},
	"enrollments": {"id", "studentId", "classId", "createdAt"},
}

type User struct{ ID, Status string }
type Class struct{ ID, Name, Program, Cohort string }
type Participation struct {
	ID, ClassID, UserID, EpisodeID string
	Roles                          []string
}
type Decision struct{ Table, SourceID, Disposition, Reason, TargetID, SourceTime string }
type Counts struct {
	Tables         map[string]int `json:"tables"`
	Candidates     int            `json:"candidate_source_records"`
	Held           int            `json:"held_source_records"`
	Deferred       int            `json:"deferred_source_records"`
	Users          int            `json:"users"`
	Suspended      int            `json:"suspended_users"`
	Classes        int            `json:"draft_classes"`
	TeacherRoles   int            `json:"teacher_roles"`
	StudentRoles   int            `json:"student_roles"`
	Mentorless     int            `json:"classes_without_mentor"`
	Participations int            `json:"participations"`
}
type Plan struct {
	Users          []User
	Classes        []Class
	Participations []Participation
	Decisions      []Decision
	Counts         Counts
}

func digest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (p Plan) Digest() string {
	data, _ := json.Marshal(p)
	return digest(data)
}

func id(kind string, parts ...string) string {
	return digest([]byte(originNamespace + "\x00" + kind + "\x00" + strings.Join(parts, "\x00")))[:32]
}

// Build accepts only the exact, reviewed SQL archive. SQL is parsed as data.
func Build(raw []byte) (Plan, error) {
	if len(raw) != SnapshotSize || digest(raw) != SnapshotHash {
		return Plan{}, ErrSource
	}
	rows, counts, err := readCopy(raw)
	if err != nil {
		return Plan{}, err
	}
	return mapCore(rows, counts)
}

type row map[string]string

func readCopy(raw []byte) (map[string][]row, map[string]int, error) {
	tables := make(map[string][]row)
	counts := make(map[string]int)
	known := make(map[string]bool)
	for _, table := range foundation.BaselineTables {
		known[table] = true
	}
	// The later staging snapshot contains this additional table.
	known["program_heads"] = true
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	var table string
	var width int
	var columns map[string]int
	for scanner.Scan() {
		line := scanner.Bytes()
		if table == "" {
			match := copyHeader.FindSubmatch(line)
			if match == nil {
				continue
			}
			table = string(match[1])
			if _, duplicate := counts[table]; duplicate || !known[table] {
				return nil, nil, ErrSource
			}
			counts[table] = 0
			columns = map[string]int{}
			parts := bytes.Split(match[2], []byte(", "))
			width = len(parts)
			for i, part := range parts {
				name := strings.Trim(string(part), "\"")
				if !columnPattern.MatchString(name) {
					return nil, nil, ErrSource
				}
				if _, duplicate := columns[name]; duplicate {
					return nil, nil, ErrSource
				}
				columns[name] = i
			}
			for _, key := range projections[table] {
				if _, exists := columns[key]; !exists {
					return nil, nil, ErrSource
				}
			}
			continue
		}
		if bytes.Equal(line, []byte(`\.`)) {
			table = ""
			continue
		}
		parts := bytes.Split(line, []byte("\t"))
		if len(parts) != width {
			return nil, nil, ErrSource
		}
		counts[table]++
		if keys := projections[table]; len(keys) != 0 {
			r := make(row, len(keys))
			for _, key := range keys {
				value, err := decodeCopy(parts[columns[key]])
				if err != nil {
					return nil, nil, err
				}
				r[key] = value
			}
			tables[table] = append(tables[table], r)
		}
	}
	if table != "" || scanner.Err() != nil {
		return nil, nil, ErrSource
	}
	total := 0
	for _, count := range counts {
		total += count
	}
	if len(counts) != 21 || total != 7843 {
		return nil, nil, ErrSource
	}
	return tables, counts, nil
}

func decodeCopy(field []byte) (string, error) {
	if bytes.Equal(field, []byte(`\N`)) {
		return "", nil
	}
	result := make([]byte, 0, len(field))
	control := map[byte]byte{'b': '\b', 'f': '\f', 'n': '\n', 'r': '\r', 't': '\t', 'v': '\v'}
	for i := 0; i < len(field); i++ {
		c := field[i]
		if c != '\\' {
			result = append(result, c)
			continue
		}
		i++
		if i >= len(field) {
			return "", ErrSource
		}
		c = field[i]
		if value, ok := control[c]; ok {
			result = append(result, value)
			continue
		}
		if c == 'x' || c >= '0' && c <= '7' {
			start, base, limit := i, 8, 3
			if c == 'x' {
				start, base, limit = i+1, 16, 2
			}
			end := start
			for end < len(field) && end-start < limit {
				if _, err := strconv.ParseUint(string(field[end:end+1]), base, 8); err != nil {
					break
				}
				end++
			}
			if end > start {
				value, _ := strconv.ParseUint(string(field[start:end]), base, 16)
				result = append(result, byte(value))
				i = end - 1
				continue
			}
		}
		result = append(result, c)
	}
	if !utf8.Valid(result) || bytes.IndexByte(result, 0) >= 0 {
		return "", ErrSource
	}
	return string(result), nil
}

func index(rows []row) (map[string]row, error) {
	result := make(map[string]row, len(rows))
	for _, r := range rows {
		key := r["id"]
		if !uuidPattern.MatchString(key) || result[key] != nil {
			return nil, ErrMapping
		}
		result[key] = r
	}
	return result, nil
}

func roles(r row) (map[string]bool, error) {
	var values []string
	if err := json.Unmarshal([]byte(r["roles"]), &values); err != nil || values == nil {
		return nil, ErrMapping
	}
	result := make(map[string]bool)
	for _, value := range values {
		if result[value] || value != "admin" && value != "facilitator" && value != "mentor" && value != "student" {
			return nil, ErrMapping
		}
		result[value] = true
	}
	return result, nil
}

func mapCore(rows map[string][]row, tables map[string]int) (Plan, error) {
	p := Plan{Counts: Counts{Tables: tables}}
	indexes := map[string]map[string]row{}
	for table := range projections {
		idx, err := index(rows[table])
		if err != nil {
			return Plan{}, err
		}
		indexes[table] = idx
		p.Counts.Candidates += len(idx)
	}
	for table, count := range tables {
		if _, core := projections[table]; !core {
			p.Counts.Deferred += count
		}
	}
	decide := func(table, key, disposition, reason, target, sourceTime string) {
		p.Decisions = append(p.Decisions, Decision{table, key, disposition, reason, target, sourceTime})
	}
	accountRoles := map[string]map[string]bool{}
	for _, user := range rows["users"] {
		key, status := user["id"], user["status"]
		if status != "active" && status != "suspended" {
			return Plan{}, ErrMapping
		}
		granted, err := roles(user)
		if err != nil {
			return Plan{}, err
		}
		accountRoles[key] = granted
		target := id("user", key)
		p.Users = append(p.Users, User{target, status})
		decide("users", key, "mapped", "historical_identity", target, "")
		if status == "suspended" {
			p.Counts.Suspended++
		}
	}
	for _, table := range []string{"programs", "batches"} {
		for _, r := range rows[table] {
			if strings.TrimSpace(r["name"]) == "" {
				return Plan{}, ErrMapping
			}
			decide(table, r["id"], "metadata", "label_source", "", "")
		}
	}
	participations := map[[2]string]*Participation{}
	addRole := func(class, user, role string) *Participation {
		key := [2]string{class, user}
		existing := participations[key]
		if existing == nil {
			existing = &Participation{ID: id("participation", class, user), ClassID: id("class", class), UserID: id("user", user), EpisodeID: id("episode", class, user)}
			participations[key] = existing
		}
		existing.Roles = append(existing.Roles, role)
		return existing
	}
	names := map[string]int{}
	for _, r := range rows["classes"] {
		key := r["id"]
		program := strings.TrimSpace(indexes["programs"][r["programId"]]["name"])
		cohort := strings.TrimSpace(indexes["batches"][r["batchId"]]["name"])
		if program == "" || cohort == "" {
			return Plan{}, ErrMapping
		}
		name := program + " — " + cohort
		names[strings.ToLower(name)]++
		p.Classes = append(p.Classes, Class{id("class", key), name, program, cohort})
		decide("classes", key, "mapped", "standalone_class", id("class", key), "")
		mentor := r["mentorId"]
		if mentor == "" {
			p.Counts.Mentorless++
			continue
		}
		if !accountRoles[mentor]["mentor"] || indexes["users"][mentor]["status"] != "active" {
			return Plan{}, ErrMapping
		}
		addRole(key, mentor, "teacher")
		p.Counts.TeacherRoles++
	}
	for i := range p.Classes {
		class := &p.Classes[i]
		if names[strings.ToLower(class.Name)] > 1 {
			class.Name += " · " + class.ID[:8]
		}
		if utf8.RuneCountInString(class.Name) > 200 {
			return Plan{}, ErrMapping
		}
	}
	pairs := map[[2]string]bool{}
	for _, r := range rows["enrollments"] {
		class, user := r["classId"], r["studentId"]
		key := [2]string{class, user}
		if indexes["classes"][class] == nil || indexes["users"][user] == nil || pairs[key] || r["createdAt"] == "" {
			return Plan{}, ErrMapping
		}
		pairs[key] = true
		if !accountRoles[user]["student"] || indexes["classes"][class]["mentorId"] == user {
			reason := "legacy_student_role_disagrees"
			if accountRoles[user]["student"] {
				reason = "teacher_student_same_class_review"
			}
			decide("enrollments", r["id"], "held", reason, "", r["createdAt"])
			p.Counts.Held++
			continue
		}
		target := addRole(class, user, "student")
		decide("enrollments", r["id"], "mapped", "participation_at_snapshot", target.ID, r["createdAt"])
		p.Counts.StudentRoles++
	}
	for _, item := range participations {
		sort.Strings(item.Roles)
		p.Participations = append(p.Participations, *item)
	}
	sort.Slice(p.Participations, func(i, j int) bool { return p.Participations[i].ID < p.Participations[j].ID })
	p.Counts.Candidates -= p.Counts.Held
	p.Counts.Classes, p.Counts.Users, p.Counts.Participations = len(p.Classes), len(p.Users), len(p.Participations)
	return p, nil
}
