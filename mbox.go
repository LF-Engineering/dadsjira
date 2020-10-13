package dads

import (
	"bytes"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	// LowerDayNames - downcased 3 letter US day names
	LowerDayNames = map[string]struct{}{
		"mon": {},
		"tue": {},
		"wed": {},
		"thu": {},
		"fri": {},
		"sat": {},
		"sun": {},
	}
	// LowerMonthNames - map lower month names
	LowerMonthNames = map[string]string{
		"jan": "Jan",
		"feb": "Feb",
		"mar": "Mar",
		"apr": "Apr",
		"may": "May",
		"jun": "Jun",
		"jul": "Jul",
		"aug": "Aug",
		"sep": "Sep",
		"oct": "Oct",
		"nov": "Nov",
		"dec": "Dec",
	}
	// SpacesRE - match 1 or more space characters
	SpacesRE = regexp.MustCompile(`\s+`)
)

// ParseMBoxMsg - parse a raw MBox message into object to be inserte dinto raw ES
func ParseMBoxMsg(ctx *Ctx, groupName string, msg []byte) (item map[string]interface{}, valid, warn bool) {
	item = make(map[string]interface{})
	raw := make(map[string][][]byte)
	addRaw := func(k string, v []byte, replace int) {
		// replace: 0-add new item, 1-replace current, 2-replace all
		// Printf("addRaw(%s,%d,%d) '%s'\n", k, len(v), replace, string(v))
		a, ok := raw[k]
		if ok {
			switch replace {
			case 0:
				raw[k] = append(a, v)
			case 1:
				l := len(a)
				raw[k][l-1] = v
			case 2:
				raw[k] = [][]byte{v}
			default:
				Printf("addRaw called with an unsupported replace mode(%s,%d)\n", groupName, len(msg))
			}
			return
		}
		raw[k] = [][]byte{v}
	}
	getRaw := func(k string) (v []byte, ok bool) {
		a, ok := raw[k]
		if !ok {
			return
		}
		v = a[len(a)-1]
		return
	}
	mustGetRaw := func(k string) (v []byte) {
		a, ok := raw[k]
		if !ok {
			return
		}
		v = a[len(a)-1]
		return
	}
	lines := bytes.Split(msg, GroupsioMsgLineSeparator)
	boundary := []byte("")
	isContinue := func(i int, line []byte) (is bool) {
		is = bytes.HasPrefix(line, []byte(" ")) || bytes.HasPrefix(line, []byte("\t"))
		return
	}
	keyRE := regexp.MustCompile(`^[\w_.-]+$`)
	getHeader := func(i int, line []byte) (key string, val []byte, ok bool) {
		sep := []byte(": ")
		ary := bytes.Split(line, sep)
		if len(ary) == 1 {
			ary := bytes.Split(line, []byte(":"))
			if len(ary) == 1 {
				return
			}
		}
		key = string(ary[0])
		if len(key) > 160 {
			return
		}
		match := keyRE.MatchString(string(key))
		if !match {
			return
		}
		val = bytes.Join(ary[1:], sep)
		ok = true
		return
	}
	getContinuation := func(i int, line []byte) (val []byte, ok bool) {
		val = bytes.TrimLeft(line, " \t")
		ok = len(val) > 0 || len(line) > 0
		return
	}
	isBoundarySep := func(i int, line []byte) (is, isEnd bool) {
		expect := []byte("--")
		expect = append(expect, boundary...)
		is = bytes.HasPrefix(line, expect)
		if is {
			isEnd = bytes.HasPrefix(line, append(expect, []byte("--")...))
		}
		return
	}
	type Body struct {
		ContentType []byte
		Properties  map[string][]byte
		Data        []byte
	}
	bodies := []Body{}
	currContentType := []byte{}
	currProperties := make(map[string][]byte)
	currData := []byte{}
	propertiesString := func(props map[string][]byte) (s string) {
		s = "{"
		ks := []string{}
		for k := range props {
			ks = append(ks, k)
		}
		if len(ks) == 0 {
			s = "{}"
			return
		}
		sort.Strings(ks)
		for _, k := range ks {
			s += k + ":" + string(props[k]) + " "
		}
		s = s[:len(s)-1] + "}"
		return
	}
	boundarySep := []byte("boundary=")
	addBody := func(i int, line []byte) (added bool) {
		if len(currContentType) == 0 || len(currData) == 0 {
			return
		}
		defer func() {
			if bytes.HasSuffix(currData, []byte("\n")) {
				currData = currData[:len(currData)-1]
			}
			if ctx.Debug > 2 {
				Printf("message(%d,%s,%s): '%s'\n", len(msg), string(currContentType), propertiesString(currProperties), string(currData))
			}
			currContentType = []byte{}
			currProperties = make(map[string][]byte)
			currData = []byte{}
		}()
		bodies = append(bodies, Body{ContentType: currContentType, Properties: currProperties, Data: currData})
		added = true
		return
	}
	savedBoundary := [][]byte{}
	savedContentType := [][]byte{}
	savedProperties := []map[string][]byte{}
	push := func(newBoundary []byte) {
		savedBoundary = append(savedBoundary, boundary)
		savedContentType = append(savedContentType, currContentType)
		savedProperties = append(savedProperties, currProperties)
		boundary = newBoundary
	}
	pop := func() {
		n := len(savedContentType) - 1
		if n < 0 {
			Printf("%s(%d): cannot pop from an empty stack\n", groupName, len(msg))
			warn = true
			return
		}
		boundary = savedBoundary[n]
		currContentType = savedContentType[n]
		currProperties = savedProperties[n]
		savedBoundary = savedBoundary[:n]
		savedContentType = savedContentType[:n]
		savedProperties = savedProperties[:n]
	}
	possibleBodyProperties := []string{ContentType, "Content-Transfer-Encoding", "Content-Language"}
	currKey := ""
	body := false
	bodyHeadersParsed := false
	nLines := len(lines)
	nSkip := 0
	var mainMultipart *bool
	for idx, line := range lines {
		if nSkip > 0 {
			nSkip--
			continue
		}
		i := idx + 2
		if idx == 0 {
			sep := []byte("\n")
			ary := bytes.Split(line, sep)
			if len(ary) > 1 {
				line = bytes.Join(ary[1:], sep)
				if len(ary[0]) > 5 {
					data := ary[0][5:]
					spaceSep := []byte(" ")
					ary2 := bytes.Split(data, spaceSep)
					if len(ary2) == 1 {
						addRaw("Mbox-From", data, 2)
					} else {
						addRaw("Mbox-From", ary2[0], 2)
						addRaw("Mbox-Date", bytes.Join(ary2[1:], spaceSep), 2)
					}
				}
			}
			line = ary[1]
		}
		if len(line) == 0 {
			if !body {
				contentType, ok := getRaw(ContentType)
				if !ok {
					contentType, ok = getRaw(LowerContentType)
					if !ok {
						contentType = []byte("text/plain")
						addRaw(LowerContentType, contentType, 0)
					}
					addRaw(ContentType, contentType, 0)
				}
				if bytes.Contains(contentType, boundarySep) {
					ary := bytes.Split(contentType, boundarySep)
					if len(ary) > 1 {
						ary2 := bytes.Split(ary[1], []byte(`"`))
						// Possibly even >= is enough here? - would fix possible buggy MBox data
						if len(ary2) > 2 {
							boundary = ary2[1]
						} else {
							ary2 := bytes.Split(ary[1], []byte(`;`))
							boundary = ary2[0]
						}
					}
					if len(boundary) == 0 {
						Printf("#%d cannot find multipart message boundary(%s,%d) '%s'\n", i, groupName, len(msg), string(contentType))
						warn = true
					}
					if mainMultipart == nil {
						dummy := true
						mainMultipart = &dummy
					}
				} else {
					currContentType = contentType
					for _, bodyProperty := range possibleBodyProperties {
						propertyVal, ok := getRaw(bodyProperty)
						if ok {
							currProperties[bodyProperty] = propertyVal
						} else {
							propertyVal, ok := getRaw(strings.ToLower(bodyProperty))
							if ok {
								currProperties[bodyProperty] = propertyVal
							}
						}
					}
					if mainMultipart == nil {
						dummy := false
						mainMultipart = &dummy
					}
					bodyHeadersParsed = true
				}
				body = true
				continue
			}
			// we could possibly assume that header is parsed when empty line is met, but this is not so simple
			if bodyHeadersParsed {
				currData = append(currData, []byte("\n")...)
			}
			continue
		}
		if body {
			// We can attempt to parse buggy mbox file - they contain header data in body - only try to find boundary separator and never fail due to this
			if len(boundary) == 0 {
				key, val, ok := getHeader(i, line)
				if ok {
					lowerKey := strings.ToLower(key)
					if lowerKey == LowerContentType {
						lIdx := idx + 1
						for {
							lI := lIdx + 2
							if lIdx >= nLines {
								break
							}
							c := isContinue(lI, lines[lIdx])
							if !c {
								break
							}
							cVal, ok := getContinuation(lI, lines[lIdx])
							if ok {
								val = append(val, cVal...)
							}
							lIdx++
							nSkip++
						}
						if bytes.Contains(val, boundarySep) {
							ary := bytes.Split(val, boundarySep)
							if len(ary) > 1 {
								ary2 := bytes.Split(ary[1], []byte(`"`))
								if len(ary2) > 2 {
									boundary = ary2[1]
								} else {
									ary2 := bytes.Split(ary[1], []byte(`;`))
									boundary = ary2[0]
								}
							}
						}
					}
				}
			}
			isBoundarySep, end := isBoundarySep(i, line)
			if isBoundarySep {
				bodyHeadersParsed = false
				_ = addBody(i, line)
				if end {
					if len(savedBoundary) > 0 {
						pop()
					}
				}
				continue
			}
			if !bodyHeadersParsed {
				key, val, ok := getHeader(i, line)
				if ok {
					lIdx := idx + 1
					for {
						lI := lIdx + 2
						if lIdx >= nLines {
							break
						}
						c := isContinue(lI, lines[lIdx])
						if !c {
							break
						}
						cVal, ok := getContinuation(lI, lines[lIdx])
						if ok {
							val = append(val, cVal...)
						}
						lIdx++
						nSkip++
					}
					lowerKey := strings.ToLower(key)
					if lowerKey == LowerContentType {
						currContentType = val
						if bytes.Contains(currContentType, boundarySep) {
							ary := bytes.Split(currContentType, boundarySep)
							if len(ary) > 1 {
								ary2 := bytes.Split(ary[1], []byte(`"`))
								if len(ary2) > 2 {
									push(ary2[1])
								} else {
									ary2 := bytes.Split(ary[1], []byte(`;`))
									push(ary2[0])
								}
							}
							if len(boundary) == 0 {
								Printf("#%d cannot find multiboundary message boundary(%s,%d)\n", i, groupName, len(msg))
								warn = true
							}
						}
						continue
					}
					currProperties[key] = val
					continue
				}
				bodyHeadersParsed = true
			}
			currData = append(currData, line...)
			continue
		}
		cont := isContinue(i, line)
		if cont {
			if currKey == "" {
				Printf("#%d no current key(%s,%d)\n", i, groupName, len(msg))
				warn = true
				break
			}
			currVal, ok := getRaw(currKey)
			if !ok {
				Printf("#%d missing %s key in %v\n", i, currKey, DumpKeys(raw))
				warn = true
				break
			}
			val, ok := getContinuation(i, line)
			if ok {
				addRaw(currKey, append(currVal, val...), 1)
				if strings.ToLower(currKey) == LowerContentType {
					addRaw(LowerContentType, mustGetRaw(currKey), 1)
				}
			}
		} else {
			key, val, ok := getHeader(i, line)
			if !ok {
				Printf("#%d incorrect header(%s,%d)\n", i, groupName, len(msg))
				warn = true
				break
			}
			// FIXME - no more needed in [][]byte raw mode?
			/*
				currVal, ok := getRaw(key)
				if ok {
					currVal = append(currVal, []byte("\n")...)
					addRaw(key, append(currVal, val...), 0)
				} else {
					addRaw(key, val, 0)
				}
			*/
			addRaw(key, val, 0)
			currKey = key
			if strings.ToLower(currKey) == LowerContentType {
				addRaw(LowerContentType, mustGetRaw(currKey), 0)
			}
		}
	}
	if len(boundary) == 0 {
		_ = addBody(nLines, []byte{})
	}
	ks := []string{}
	for k := range raw {
		lk := strings.ToLower(k)
		sv := string(mustGetRaw(k))
		item[k] = sv
		if (lk == "message-id" || lk == "date") && lk != k {
			item[lk] = sv
			ks = append(ks, lk)
		}
		if lk == "received" && lk != k {
			raw[lk] = raw[k]
		}
		ks = append(ks, k)
	}
	if ctx.Debug > 2 {
		sort.Strings(ks)
		for i, k := range ks {
			Printf("#%d %s: %s\n", i+1, k, item[k])
		}
		for i, body := range bodies {
			Printf("#%d: %s %s %d\n", i, string(body.ContentType), propertiesString(body.Properties), len(body.Data))
		}
	}
	mid, ok := item["message-id"]
	if !ok {
		Printf("%s(%d): missing Message-ID field\n", groupName, len(msg))
		return
	}
	item["Message-ID"] = mid
	var dt time.Time
	found := false
	mdt, ok := item["date"]
	if !ok {
		rcvs, ok := raw["received"]
		if !ok {
			Printf("%s(%d): missing Date & Received fields\n", groupName, len(msg))
		}
		var dts []time.Time
		for _, rcv := range rcvs {
			ary := strings.Split(string(rcv), ";")
			sdt := ary[len(ary)-1]
			dt, ok := ParseMBoxDate(sdt)
			if ok {
				dts = append(dts, dt)
			}
		}
		nDts := len(dts)
		if nDts == 0 {
			Printf("%s(%d): missing Date field and cannot parse date from Received field(s)\n", groupName, len(msg))
			return
		}
		if nDts > 1 {
			sort.Slice(dts, func(i, j int) bool { return dts[i].After(dts[j]) })
		}
		dt = dts[0]
		found = true
	}
	if !found {
		dt, ok = ParseMBoxDate(mdt.(string))
		if !ok {
			Printf("%s(%d): unable to parse date from '%s'\n", groupName, len(msg), mdt)
			return
		}
	}
	//Printf("dt=%v\n", dt)
	item["Date"] = dt
	// FIXME: continue
	// valid = true
	return
}

// ParseMBoxDate - try to parse mbox date
func ParseMBoxDate(sdt string) (dt time.Time, valid bool) {
	// https://www.broobles.com/eml2mbox/mbox.html
	// but the real world is not that simple
	for _, r := range []string{">", "\t", ",", ")", "("} {
		sdt = strings.Replace(sdt, r, "", -1)
	}
	for _, split := range []string{"+0", "+1", "."} {
		ary := strings.Split(sdt, split)
		sdt = ary[0]
	}
	for _, split := range []string{"-0", "-1"} {
		ary := strings.Split(sdt, split)
		lAry := len(ary)
		if lAry > 1 {
			_, err := strconv.Atoi(ary[lAry-1])
			if err == nil {
				sdt = strings.Join(ary[:lAry-1], split)
			}
		}
	}
	sdt = SpacesRE.ReplaceAllString(sdt, " ")
	sdt = strings.ToLower(strings.TrimSpace(sdt))
	ary := strings.Split(sdt, " ")
	day := ary[0]
	if len(day) > 3 {
		day = day[:3]
	}
	_, ok := LowerDayNames[day]
	if ok {
		sdt = strings.Join(ary[1:], " ")
	}
	sdt = strings.TrimSpace(sdt)
	for lm, m := range LowerMonthNames {
		sdt = strings.Replace(sdt, lm, m, -1)
	}
	ary = strings.Split(sdt, " ")
	if len(ary) > 4 {
		sdt = strings.Join(ary[:4], " ")
	}
	formats := []string{
		"2 Jan 2006 15:04:05",
		"02 Jan 2006 15:04:05",
		"2 Jan 06 15:04:05",
		"02 Jan 06 15:04:05",
		"2 Jan 2006 15:04",
		"02 Jan 2006 15:04",
		"2 Jan 06 15:04",
		"02 Jan 06 15:04",
		"2006-01-02 15:04:05",
	}
	var (
		err  error
		errs []error
	)
	for _, format := range formats {
		dt, err = time.Parse(format, sdt)
		if err == nil {
			// Printf("Parsed %v\n", dt)
			valid = true
			return
		}
		errs = append(errs, err)
	}
	Printf("errors: %+v\n", errs)
	Printf("sdt: %s, day: %s\n", sdt, day)
	return
}
