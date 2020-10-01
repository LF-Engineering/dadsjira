package dads

import (
	"bytes"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	jsoniter "github.com/json-iterator/go"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Typical run:
// DA_DS=jira DA_JIRA_ENRICH=1 DA_JIRA_ES_URL=... DA_JIRA_RAW_INDEX=proj-raw DA_JIRA_RICH_INDEX=proj DA_JIRA_URL=https://jira.xyz.org DA_JIRA_DEBUG=1 DA_JIRA_PROJECT=proj DA_JIRA_DB_NAME=db DA_JIRA_DB_USER=u DA_JIRA_DB_PASS=p DA_JIRA_MULTI_ORIGIN=1 ./dads

const (
	// BulkRefreshMode - bulk upload refresh mode, can be: false, true, wait_for
	BulkRefreshMode = "true"
	// KeywordMaxlength - max description length
	KeywordMaxlength = 1000
	// MultiOrgNames - suffix for multiple orgs affiliation data
	MultiOrgNames = "_multi_org_names"
)

var (
	// MappingNotAnalyzeString - make all string keywords by default (not analyze them)
	MappingNotAnalyzeString = []byte(`{"dynamic_templates":[{"notanalyzed":{"match":"*","match_mapping_type":"string","mapping":{"type":"keyword"}}},{"formatdate":{"match":"*","match_mapping_type":"date","mapping":{"type":"date","format":"strict_date_optional_time||epoch_millis"}}}]}`)
	// RawFields - standard raw fields
	RawFields = []string{DefaultDateField, DefaultTimestampField, DefaultOriginField, DefaultTagField, UUID, Offset}
)

// DS - interface for all data source types
type DS interface {
	ParseArgs(*Ctx) error
	Name() string
	Info() string
	Validate() error
	FetchRaw(*Ctx) error
	FetchItems(*Ctx) error
	Enrich(*Ctx) error
	DateField(*Ctx) string
	OffsetField(*Ctx) string
	OriginField(*Ctx) string
	Categories() map[string]struct{}
	CustomFetchRaw() bool
	CustomEnrich() bool
	SupportDateFrom() bool
	SupportOffsetFrom() bool
	ResumeNeedsOrigin(*Ctx) bool
	Origin(*Ctx) string
	ItemID(interface{}) string
	ItemUpdatedOn(interface{}) time.Time
	ItemCategory(interface{}) string
	SearchFields() map[string][]string
	ElasticRawMapping() []byte
	ElasticRichMapping() []byte
	GetItemIdentities(interface{}) (map[[3]string]struct{}, error)
	EnrichItem(map[string]interface{}, string, bool) (map[string]interface{}, error)
	AffsItems(map[string]interface{}, []string, interface{}) (map[string]interface{}, error)
	GetRoleIdentity(map[string]interface{}, string) map[string]interface{}
}

// UUIDNonEmpty - generate UUID of string args (all must be non-empty)
func UUIDNonEmpty(ctx *Ctx, args ...string) (h string) {
	if ctx.Debug > 1 {
		defer func() {
			Printf("UUIDNonEmpty(%v) --> %s\n", args, h)
		}()
	}
	stripF := func(str string) string {
		isOk := func(r rune) bool {
			return r < 32 || r >= 127
		}
		t := transform.Chain(norm.NFKD, transform.RemoveFunc(isOk))
		str, _, _ = transform.String(t, str)
		return str
	}
	arg := ""
	for _, a := range args {
		if a == "" {
			Fatalf("UUIDNonEmpty(%v) - empty argument(s) not allowed", args)
		}
		if arg != "" {
			arg += ":"
		}
		arg += stripF(a)
	}
	hash := sha1.New()
	if ctx.Debug > 1 {
		Printf("UUIDNonEmpty(%s)\n", arg)
	}
	_, err := hash.Write([]byte(arg))
	FatalOnError(err)
	h = hex.EncodeToString(hash.Sum(nil))
	return
}

// UUIDAffs - generate UUID of string args
// downcases arguments, all but first can be empty
// if argument is Nil "<nil>" replaces with "None"
func UUIDAffs(ctx *Ctx, args ...string) (h string) {
	if ctx.Debug > 1 {
		defer func() {
			Printf("UUIDAffs(%v) --> %s\n", args, h)
		}()
	}
	stripF := func(str string) string {
		isOk := func(r rune) bool {
			return r < 32 || r >= 127
		}
		t := transform.Chain(norm.NFKD, transform.RemoveFunc(isOk))
		str, _, _ = transform.String(t, str)
		return str
	}
	arg := ""
	for i, a := range args {
		if i == 0 && a == "" {
			Fatalf("UUIDAffs(%v) - empty first argument not allowed", args)
		}
		if a == Nil {
			a = None
		}
		if arg != "" {
			arg += ":"
		}
		arg += stripF(a)
	}
	hash := sha1.New()
	if ctx.Debug > 1 {
		Printf("UUIDAffs(%s)\n", strings.ToLower(arg))
	}
	_, err := hash.Write([]byte(strings.ToLower(arg)))
	FatalOnError(err)
	h = hex.EncodeToString(hash.Sum(nil))
	return
}

// KeysOnly - return a corresponding interface contining only keys
func KeysOnly(i interface{}) (o map[string]interface{}) {
	if i == nil {
		return
	}
	is, ok := i.(map[string]interface{})
	if !ok {
		return
	}
	o = make(map[string]interface{})
	for k, v := range is {
		o[k] = KeysOnly(v)
	}
	return
}

// DumpKeys - dump interface structure, but only keys, no values
func DumpKeys(i interface{}) string {
	return strings.Replace(fmt.Sprintf("%v", KeysOnly(i)), "map[]", "", -1)
}

// PartitionString - partition a string to [pre-sep, sep, post-sep]
func PartitionString(s string, sep string) [3]string {
	parts := strings.SplitN(s, sep, 2)
	if len(parts) == 1 {
		return [3]string{parts[0], "", ""}
	}
	return [3]string{parts[0], sep, parts[1]}
}

// Dig interface for array of keys
func Dig(iface interface{}, keys []string, fatal, silent bool) (v interface{}, ok bool) {
	miss := false
	defer func() {
		if !ok && fatal {
			Fatalf("cannot dig %+v in %s", keys, DumpKeys(iface))
		}
	}()
	item, o := iface.(map[string]interface{})
	if !o {
		Printf("Interface cannot be parsed: %+v\n", iface)
		return
	}
	last := len(keys) - 1
	for i, key := range keys {
		var o bool
		if i < last {
			item, o = item[key].(map[string]interface{})
		} else {
			v, o = item[key]
		}
		if !o {
			if !silent {
				Printf("%+v, current: %s, %d/%d failed\n", keys, key, i+1, last+1)
			}
			miss = true
			break
		}
	}
	ok = !miss
	return
}

// Request - wrapper to do any HTTP request
// jsonStatuses - set of status code ranges to be parsed as JSONs
// errorStatuses - specify status value ranges for which we should return error
// okStatuses - specify status value ranges for which we should return error (only taken into account if not empty)
func Request(
	ctx *Ctx,
	url, method string,
	headers map[string]string,
	payload []byte,
	jsonStatuses, errorStatuses, okStatuses map[[2]int]struct{},
) (result interface{}, status int, err error) {
	var (
		payloadBody *bytes.Reader
		req         *http.Request
	)
	if len(payload) > 0 {
		payloadBody = bytes.NewReader(payload)
		req, err = http.NewRequest(method, url, payloadBody)
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		err = fmt.Errorf("new request error:%+v for method:%s url:%s payload:%s", err, method, url, string(payload))
		return
	}
	for header, value := range headers {
		req.Header.Set(header, value)
	}
	var resp *http.Response
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		err = fmt.Errorf("do request error:%+v for method:%s url:%s headers:%v payload:%s", err, method, url, headers, string(payload))
		return
	}
	var body []byte
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		err = fmt.Errorf("read request body error:%+v for method:%s url:%s headers:%v payload:%s", err, method, url, headers, string(payload))
		return
	}
	_ = resp.Body.Close()
	status = resp.StatusCode
	hit := false
	for r := range jsonStatuses {
		if status >= r[0] && status <= r[1] {
			hit = true
			break
		}
	}
	if hit {
		err = jsoniter.Unmarshal(body, &result)
		if err != nil {
			err = fmt.Errorf("unmarshall request error:%+v for method:%s url:%s headers:%v status:%d payload:%s body:%s", err, method, url, headers, status, string(payload), string(body))
			return
		}
	} else {
		result = body
	}
	hit = false
	for r := range errorStatuses {
		if status >= r[0] && status <= r[1] {
			hit = true
			break
		}
	}
	if hit {
		err = fmt.Errorf("status error:%+v for method:%s url:%s headers:%v status:%d payload:%s body:%s result:%+v", err, method, url, headers, status, string(payload), string(body), result)
	}
	if len(okStatuses) > 0 {
		hit = false
		for r := range okStatuses {
			if status >= r[0] && status <= r[1] {
				hit = true
				break
			}
		}
		if !hit {
			err = fmt.Errorf("status not success:%+v for method:%s url:%s headers:%v status:%d payload:%s body:%s result:%+v", err, method, url, headers, status, string(payload), string(body), result)
		}
	}
	return
}

// SendToElastic - send items to ElasticSearch
func SendToElastic(ctx *Ctx, ds DS, raw bool, key string, items []interface{}) (err error) {
	if ctx.Debug > 0 {
		Printf("%s: saving %d items\n", ds.Name(), len(items))
	}
	var url string
	if raw {
		url = ctx.ESURL + "/" + ctx.RawIndex + "/_bulk?refresh=" + BulkRefreshMode
	} else {
		url = ctx.ESURL + "/" + ctx.RichIndex + "/_bulk?refresh=" + BulkRefreshMode
	}
	// {"index":{"_id":"uuid"}}
	payloads := []byte{}
	newLine := []byte("\n")
	var (
		doc []byte
		hdr []byte
	)
	for _, item := range items {
		doc, err = jsoniter.Marshal(item)
		if err != nil {
			return
		}
		uuid, ok := item.(map[string]interface{})[key].(string)
		if !ok {
			err = fmt.Errorf("missing %s property in %+v", key, DumpKeys(item))
			return
		}
		hdr = []byte(`{"index":{"_id":"` + uuid + "\"}}\n")
		payloads = append(payloads, hdr...)
		payloads = append(payloads, doc...)
		payloads = append(payloads, newLine...)
	}
	_, _, err = Request(
		ctx,
		url,
		Post,
		map[string]string{"Content-Type": "application/x-ndjson"},
		payloads,
		nil,                                 // JSON statuses
		map[[2]int]struct{}{{400, 599}: {}}, // error statuses: 400-599
		nil,                                 // OK statuses
	)
	if err == nil {
		if ctx.Debug > 0 {
			Printf("%s: saved %d items\n", ds.Name(), len(items))
		}
		return
	}
	Printf("%s: bulk upload of %d items failed, falling back to one-by-one mode\n", ds.Name(), len(items))
	if ctx.Debug > 1 {
		Printf("Error: %+v\n", err)
	}
	err = nil
	// Fallback to one-by-one inserts
	if raw {
		url = ctx.ESURL + "/" + ctx.RawIndex + "/_doc/"
	} else {
		url = ctx.ESURL + "/" + ctx.RichIndex + "/_doc/"
	}
	headers := map[string]string{"Content-Type": "application/json"}
	for _, item := range items {
		doc, _ = jsoniter.Marshal(item)
		uuid, _ := item.(map[string]interface{})[key].(string)
		_, _, err = Request(
			ctx,
			url+uuid,
			Put,
			headers,
			doc,
			nil,                                 // JSON statuses
			map[[2]int]struct{}{{400, 599}: {}}, // error statuses: 400-599
			map[[2]int]struct{}{{200, 201}: {}}, // OK statuses: 200-201
		)
	}
	if ctx.Debug > 0 {
		Printf("%s: saved %d items (in non-bulk mode)\n", ds.Name(), len(items))
	}
	return
}

// GetLastUpdate - get last update date from ElasticSearch
func GetLastUpdate(ctx *Ctx, ds DS, raw bool) (lastUpdate *time.Time) {
	// curl -s -XPOST -H 'Content-type: application/json' '${URL}/index/_search?size=0' -d '{"aggs":{"m":{"max":{"field":"date_field"}}}}' | jq -r '.aggregations.m.value_as_string'
	dateField := JSONEscape(ds.DateField(ctx))
	originField := JSONEscape(ds.OriginField(ctx))
	origin := JSONEscape(ds.Origin(ctx))
	var payloadBytes []byte
	if ds.ResumeNeedsOrigin(ctx) {
		payloadBytes = []byte(`{"query":{"bool":{"filter":{"term":{"` + originField + `":"` + origin + `"}}}},"aggs":{"m":{"max":{"field":"` + dateField + `"}}}}`)
	} else {
		payloadBytes = []byte(`{"aggs":{"m":{"max":{"field":"` + dateField + `"}}}}`)
	}
	var url string
	if raw {
		url = ctx.ESURL + "/" + ctx.RawIndex + "/_search?size=0"
	} else {
		url = ctx.ESURL + "/" + ctx.RichIndex + "/_search?size=0"
	}
	if ctx.Debug > 0 {
		Printf("raw %v resume from date query: %s\n", raw, string(payloadBytes))
	}
	method := Post
	resp, _, err := Request(
		ctx,
		url,
		method,
		map[string]string{"Content-Type": "application/json"}, // headers
		payloadBytes,                        // payload
		nil,                                 // JSON statuses
		nil,                                 // Error statuses
		map[[2]int]struct{}{{200, 200}: {}}, // OK statuses: 200, 404
	)
	FatalOnError(err)
	type resultStruct struct {
		Aggs struct {
			M struct {
				Str string `json:"value_as_string"`
			} `json:"m"`
		} `json:"aggregations"`
	}
	var res resultStruct
	err = jsoniter.Unmarshal(resp.([]byte), &res)
	if err != nil {
		Printf("JSON decode error: %+v for %s url: %s, query: %s\n", err, method, url, string(payloadBytes))
		return
	}
	if res.Aggs.M.Str != "" {
		var tm time.Time
		tm, err = TimeParseAny(res.Aggs.M.Str)
		if err != nil {
			Printf("Decode aggregations error: %+v for %s url: %s, query: %s\n", err, method, url, string(payloadBytes))
			return
		}
		lastUpdate = &tm
	}
	return
}

// GetLastOffset - get last offset from ElasticSearch
func GetLastOffset(ctx *Ctx, ds DS, raw bool) (offset float64) {
	offset = -1.0
	// curl -s -XPOST -H 'Content-type: application/json' '${URL}/index/_search?size=0' -d '{"aggs":{"m":{"max":{"field":"offset_field"}}}}' | jq -r '.aggregations.m.value'
	offsetField := JSONEscape(ds.OffsetField(ctx))
	originField := JSONEscape(ds.OffsetField(ctx))
	origin := JSONEscape(ds.Origin(ctx))
	var payloadBytes []byte
	if ds.ResumeNeedsOrigin(ctx) {
		payloadBytes = []byte(`{"query":{"bool":{"filter":{"term":{"` + originField + `":"` + origin + `"}}}},"aggs":{"m":{"max":{"field":"` + offsetField + `"}}}}`)
	} else {
		payloadBytes = []byte(`{"aggs":{"m":{"max":{"field":"` + offsetField + `"}}}}`)
	}
	var url string
	if raw {
		url = ctx.ESURL + "/" + ctx.RawIndex + "/_search?size=0"
	} else {
		url = ctx.ESURL + "/" + ctx.RichIndex + "/_search?size=0"
	}
	if ctx.Debug > 0 {
		Printf("raw %v resume from offset query: %s\n", raw, string(payloadBytes))
	}
	method := Post
	resp, _, err := Request(
		ctx,
		url,
		method,
		map[string]string{"Content-Type": "application/json"}, // headers
		payloadBytes,                        // payload
		nil,                                 // JSON statuses
		nil,                                 // Error statuses
		map[[2]int]struct{}{{200, 200}: {}}, // OK statuses: 200, 404
	)
	FatalOnError(err)
	type resultStruct struct {
		Aggs struct {
			M struct {
				Int *float64 `json:"value,omitempty"`
			} `json:"m"`
		} `json:"aggregations"`
	}
	var res = resultStruct{}
	err = jsoniter.Unmarshal(resp.([]byte), &res)
	if err != nil {
		Printf("JSON decode error: %+v for %s url: %s, query: %s\n", err, method, url, string(payloadBytes))
		return
	}
	if res.Aggs.M.Int != nil {
		offset = *res.Aggs.M.Int
	}
	return
}

// CommonFields - common rich item fields
func CommonFields(ds DS, date interface{}, category string) (fields map[string]interface{}) {
	var dt *time.Time
	sDate, ok := date.(string)
	if ok {
		d, err := TimeParseES(sDate)
		if err == nil {
			dt = &d
		}
	}
	name := "is_" + ds.Name() + "_" + category
	fields = map[string]interface{}{"grimoire_creation_date": dt, name: 1}
	return
}

// EmptyAffsItem - return empty affiliation sitem for a given role
func EmptyAffsItem(role string) map[string]interface{} {
	return map[string]interface{}{
		role + "_id":         "",
		role + "_uuid":       "",
		role + "_name":       "",
		role + "_user_name":  "",
		role + "_domain":     "",
		role + "_gender":     "",
		role + "_gender_acc": nil,
		role + "_org_name":   "",
		role + "_bot":        false,
		role + MultiOrgNames: []interface{}{},
	}
}

// IdenityAffsData - add affiliations related data
func IdenityAffsData(identity map[string]interface{}, dt time.Time, role string) (outItem map[string]interface{}) {
	outItem = EmptyAffsItem(role)
	return
}

// UploadIdentities - upload identities to SH DB
func UploadIdentities(ctx *Ctx, ds DS) (err error) {
	uploadFunc := func(docs, outDocs *[]interface{}) (e error) {
		var tx *sql.Tx
		e = SetDBSessionOrigin(ctx)
		if e != nil {
			return
		}
		tx, e = ctx.DB.Begin()
		if e != nil {
			return
		}
		nIdents := len(*docs)
		defer func() {
			if tx != nil {
				Printf("Rolling back %d items\n", nIdents)
				_ = tx.Rollback()
			}
		}()
		if ctx.Debug > 0 {
			Printf("Bulk adding %d idents\n", nIdents)
		}
		bulkSize := ctx.DBBulkSize / 6
		nPacks := nIdents / bulkSize
		if nIdents%bulkSize != 0 {
			nPacks++
		}
		source := ds.Name()
		for i := 0; i < nPacks; i++ {
			from := i * bulkSize
			to := from + bulkSize
			if to > nIdents {
				to = nIdents
			}
			queryU := "insert ignore into uidentities(uuid, last_modified) values"
			queryP := "insert ignore into profiles(uuid) values"
			queryI := "insert ignore into identities(id, source, name, email, username, uuid, last_modified) values"
			argsU := []interface{}{}
			argsP := []interface{}{}
			argsI := []interface{}{}
			if ctx.Debug > 0 {
				Printf("Bulk adding pack #%d %d-%d (%d/%d)\n", i+1, from, to, to-from, nIdents)
			}
			for j := from; j < to; j++ {
				ident, _ := (*docs)[j].([3]string)
				name := ident[0]
				username := ident[1]
				email := ident[2]
				// uuid(source, email, name, username)
				uuid := UUIDAffs(ctx, source, email, name, username)
				queryU += fmt.Sprintf("(?,now()),")
				argsU = append(argsU, uuid)
				queryP += fmt.Sprintf("(?),")
				argsP = append(argsP, uuid)
				var (
					pname     *string
					pemail    *string
					pusername *string
				)
				if name != Nil {
					pname = &name
				}
				if email != Nil {
					pemail = &email
				}
				if username != Nil {
					pusername = &username
				}
				queryI += fmt.Sprintf("(?,?,?,?,?,?,now()),")
				argsI = append(argsI, uuid, source, pname, pemail, pusername, uuid)
			}
			queryU = queryU[:len(queryU)-1]
			queryP = queryP[:len(queryP)-1]
			queryI = queryI[:len(queryI)-1]
			_, e = ExecSQL(ctx, tx, queryU, argsU...)
			if e != nil {
				return
			}
			_, e = ExecSQL(ctx, tx, queryP, argsP...)
			if e != nil {
				return
			}
			_, e = ExecSQL(ctx, tx, queryI, argsI...)
			if e != nil {
				return
			}
		}
		e = tx.Commit()
		if e != nil {
			return
		}
		*docs = []interface{}{}
		tx = nil
		return
	}
	itemsFunc := func(items []interface{}, docs *[]interface{}) (e error) {
		idents := make(map[[3]string]struct{})
		for _, doc := range *docs {
			idents[doc.([3]string)] = struct{}{}
		}
		for _, item := range items {
			doc, ok := item.(map[string]interface{})["_source"]
			if !ok {
				err = fmt.Errorf("Missing _source in item %+v", DumpKeys(item))
				return
			}
			var identities map[[3]string]struct{}
			identities, err = ds.GetItemIdentities(doc)
			if err != nil {
				err = fmt.Errorf("Cannot get identities from doc %+v", DumpKeys(doc))
				return
			}
			if identities == nil {
				continue
			}
			for identity := range identities {
				idents[identity] = struct{}{}
			}
		}
		*docs = []interface{}{}
		for ident := range idents {
			*docs = append(*docs, ident)
		}
		return
	}
	err = ForEachRawItem(ctx, ds, ctx.DBBulkSize, uploadFunc, itemsFunc)
	return
}

// EnrichItems - perform the enrichment
func EnrichItems(ctx *Ctx, ds DS) (err error) {
	dbConfigured := ctx.AffsDBConfigured()
	enrichFunc := func(docs, outDocs *[]interface{}) (e error) {
		Printf("-> enrichFunc(%d,%d)\n", len(*docs), len(*outDocs))
		var rich map[string]interface{}
		for _, doc := range *docs {
			item, ok := doc.(map[string]interface{})
			if !ok {
				e = fmt.Errorf("Failed to parse document %+v\n", doc)
				return
			}
			for _, author := range []string{"creator", "assignee", "reporter"} {
				rich, e = ds.EnrichItem(item, author, dbConfigured)
				if e != nil {
					return
				}
				// should detect if a particular author type is missing
				*outDocs = append(*outDocs, rich)
			}
		}
		*docs = []interface{}{}
		Printf("<- enrichFunc(%d,%d)\n", len(*docs), len(*outDocs))
		return
	}
	itemsFunc := func(items []interface{}, docs *[]interface{}) (e error) {
		for _, item := range items {
			doc, ok := item.(map[string]interface{})["_source"]
			if !ok {
				e = fmt.Errorf("Missing _source in item %+v", DumpKeys(item))
				return
			}
			*docs = append(*docs, doc)
		}
		return
	}
	err = ForEachRawItem(ctx, ds, ctx.ESBulkSize, enrichFunc, itemsFunc)
	return
}

// ForEachRawItem - perform specific function for all raw items
func ForEachRawItem(ctx *Ctx, ds DS, packSize int, ufunct func(*[]interface{}, *[]interface{}) error, uitems func([]interface{}, *[]interface{}) error) (err error) {
	dateField := JSONEscape(ds.DateField(ctx))
	originField := JSONEscape(ds.OriginField(ctx))
	origin := JSONEscape(ds.Origin(ctx))
	var (
		scroll   *string
		dateFrom string
		res      interface{}
		status   int
	)
	headers := map[string]string{"Content-Type": "application/json"}
	if ctx.DateFrom != nil {
		dateFrom = ToESDate(*ctx.DateFrom)
	}
	attemptAt := time.Now()
	total := 0
	// Defer free scroll
	defer func() {
		if scroll == nil {
			return
		}
		url := ctx.ESURL + "/_search/scroll"
		payload := []byte(`{"scroll_id":"` + *scroll + `"}`)
		_, _, err := Request(
			ctx,
			url,
			Delete,
			headers,
			payload,
			nil,
			nil,                                 // Error statuses
			map[[2]int]struct{}{{200, 200}: {}}, // OK statuses
		)
		if err != nil {
			Printf("Error releasing scroll %s: %+v\n", *scroll, err)
		}
	}()
	thrN := GetThreadsNum(ctx)
	nThreads := 0
	var (
		mtx *sync.Mutex
		ch  chan error
	)
	docs := []interface{}{}
	outDocs := []interface{}{}
	if thrN > 1 {
		mtx = &sync.Mutex{}
		ch = make(chan error)
	}
	funct := func(c chan error) (e error) {
		defer func() {
			if thrN > 1 {
				mtx.Unlock()
			}
			if c != nil {
				c <- e
			}
		}()
		if thrN > 1 {
			mtx.Lock()
		}
		e = ufunct(&docs, &outDocs)
		return
	}
	needsOrigin := ds.ResumeNeedsOrigin(ctx)
	for {
		var (
			url     string
			payload []byte
		)
		if scroll == nil {
			url = ctx.ESURL + "/" + ctx.RawIndex + "/_search?scroll=" + ctx.ESScrollWait + "&size=" + strconv.Itoa(ctx.ESScrollSize)
			if needsOrigin {
				if ctx.DateFrom == nil {
					payload = []byte(`{"query":{"bool":{"filter":{"term":{"` + originField + `":"` + origin + `"}}}},"sort":{"` + dateField + `":{"order":"asc"}}}`)
				} else {
					payload = []byte(`{"query":{"bool":{"filter":[{"term":{"` + originField + `":"` + origin + `"}},{"range":{"` + dateField + `":{"gte":"` + dateFrom + `"}}}]}},"sort":{"` + dateField + `":{"order":"asc"}}}`)
				}
			} else {
				if ctx.DateFrom == nil {
					payload = []byte(`{"sort":{"` + dateField + `":{"order":"asc"}}}`)
				} else {
					payload = []byte(`{"query":{"bool":{"range":{"` + dateField + `":{"gte":"` + dateFrom + `"}}}},"sort":{"` + dateField + `":{"order":"asc"}}}`)
					payload = []byte(`{"query":{"bool":{"filter":{"range":{"` + dateField + `":{"gte":"` + dateFrom + `"}}}}},"sort":{"` + dateField + `":{"order":"asc"}}}`)
				}
			}
			if ctx.Debug > 0 {
				Printf("processing query: %s\n", string(payload))
			}
		} else {
			url = ctx.ESURL + "/_search/scroll"
			payload = []byte(`{"scroll":"` + ctx.ESScrollWait + `","scroll_id":"` + *scroll + `"}`)
		}
		res, status, err = Request(
			ctx,
			url,
			Post,
			headers,
			payload,
			map[[2]int]struct{}{{200, 200}: {}}, // JSON statuses
			nil,                                 // Error statuses
			map[[2]int]struct{}{{200, 200}: {}, {500, 500}: {}}, // OK statuses
		)
		FatalOnError(err)
		if scroll == nil && status == 500 && strings.Contains(string(res.([]byte)), TooManyScrolls) {
			time.Sleep(5)
			now := time.Now()
			elapsed := now.Sub(attemptAt)
			Printf("%d Retrying scroll, first attempt at %+v, elapsed %+v/%.0fs\n", len(res.(map[string]interface{})), attemptAt, elapsed, ctx.ESScrollWaitSecs)
			if elapsed.Seconds() > ctx.ESScrollWaitSecs {
				Fatalf("Tried to acquire scroll too many times, first attempt at %v, elapsed %v/%.0fs", attemptAt, elapsed, ctx.ESScrollWaitSecs)
			}
			continue
		}
		sScroll, ok := res.(map[string]interface{})["_scroll_id"].(string)
		if !ok {
			err = fmt.Errorf("Missing _scroll_id in the response %+v", DumpKeys(res))
			return
		}
		scroll = &sScroll
		items, ok := res.(map[string]interface{})["hits"].(map[string]interface{})["hits"].([]interface{})
		if !ok {
			err = fmt.Errorf("Missing hits.hits in the response %+v", DumpKeys(res))
			return
		}
		nItems := len(items)
		if nItems == 0 {
			break
		}
		if ctx.Debug > 0 {
			Printf("Processing %d items\n", nItems)
		}
		if thrN > 1 {
			mtx.Lock()
		}
		err = uitems(items, &docs)
		if err != nil {
			return
		}
		nDocs := len(docs)
		if nDocs >= packSize {
			if thrN > 1 {
				go func() {
					_ = funct(ch)
				}()
				nThreads++
				if nThreads == thrN {
					err = <-ch
					if err != nil {
						return
					}
					nThreads--
				}
			} else {
				err = funct(nil)
				if err != nil {
					return
				}
			}
		}
		if thrN > 1 {
			mtx.Unlock()
		}
		total += nItems
	}
	if thrN > 1 {
		mtx.Lock()
	}
	nDocs := len(docs)
	if nDocs > 0 {
		if thrN > 1 {
			go func() {
				_ = funct(ch)
			}()
			nThreads++
			if nThreads == thrN {
				err = <-ch
				if err != nil {
					return
				}
				nThreads--
			}
		} else {
			err = funct(nil)
			if err != nil {
				return
			}
		}
	}
	if thrN > 1 {
		mtx.Unlock()
	}
	for thrN > 1 && nThreads > 0 {
		err = <-ch
		nThreads--
		if err != nil {
			return
		}
	}
	if ctx.Debug > 0 {
		Printf("Total number of items processed: %d\n", total)
	}
	return
}

// HandleMapping - create/update mapping for raw or rich index
func HandleMapping(ctx *Ctx, ds DS, raw bool) (err error) {
	// Create index, ignore if exists (see status 400 is not in error statuses)
	var url string
	if raw {
		url = ctx.ESURL + "/" + ctx.RawIndex
	} else {
		url = ctx.ESURL + "/" + ctx.RichIndex
	}
	_, _, err = Request(
		ctx,
		url,
		Put,
		nil,                                 // headers
		[]byte{},                            // payload
		nil,                                 // JSON statuses
		map[[2]int]struct{}{{401, 599}: {}}, // error statuses: 401-599
		nil,                                 // OK statuses
	)
	FatalOnError(err)
	// DS specific raw index mapping
	var mapping []byte
	if raw {
		mapping = ds.ElasticRawMapping()
	} else {
		mapping = ds.ElasticRichMapping()
	}
	url += "/_mapping"
	_, _, err = Request(
		ctx,
		url,
		Put,
		map[string]string{"Content-Type": "application/json"},
		mapping,
		nil,
		nil,
		map[[2]int]struct{}{{200, 200}: {}},
	)
	FatalOnError(err)
	// Global not analyze string mapping
	_, _, err = Request(
		ctx,
		url,
		Put,
		map[string]string{"Content-Type": "application/json"},
		MappingNotAnalyzeString,
		nil,
		nil,
		map[[2]int]struct{}{{200, 200}: {}},
	)
	FatalOnError(err)
	return
}

// FetchRaw - implement fetch raw data (generic)
func FetchRaw(ctx *Ctx, ds DS) (err error) {
	err = HandleMapping(ctx, ds, true)
	if err != nil {
		Fatalf(ds.Name()+": HandleMapping error: %+v\n", err)
	}
	if ds.CustomFetchRaw() {
		return ds.FetchRaw(ctx)
	}
	if ctx.DateFrom != nil && ctx.OffsetFrom >= 0.0 {
		Fatalf(ds.Name() + ": you cannot use both date from and offset from\n")
	}
	if ctx.DateTo != nil && ctx.OffsetTo >= 0.0 {
		Fatalf(ds.Name() + ": you cannot use both date to and offset to\n")
	}
	var (
		lastUpdate *time.Time
		offset     *float64
	)
	if ds.SupportDateFrom() {
		lastUpdate = ctx.DateFrom
		if lastUpdate == nil {
			lastUpdate = GetLastUpdate(ctx, ds, true)
		}
		if lastUpdate != nil {
			if ctx.DateFrom == nil {
				ctx.DateFromDetected = true
			}
			Printf("%s: raw: starting from date: %v, detected: %v\n", ds.Name(), *lastUpdate, ctx.DateFromDetected)
			ctx.DateFrom = lastUpdate
		} else {
			Printf("%s: raw: no start date detected\n", ds.Name())
		}
	}
	if ds.SupportOffsetFrom() {
		if ctx.OffsetFrom >= 0.0 {
			offset = &ctx.OffsetFrom
		}
		if offset == nil {
			lastOffset := GetLastOffset(ctx, ds, true)
			if lastOffset >= 0.0 {
				offset = &lastOffset
			}
		}
		if offset != nil {
			if ctx.OffsetFrom < 0.0 {
				ctx.OffsetFromDetected = true
			}
			Printf("%s: raw: starting from offset: %v, detected: %v\n", ds.Name(), *offset, ctx.OffsetFromDetected)
			ctx.OffsetFrom = *offset
		} else {
			Printf("%s: raw: no start offset detected\n", ds.Name())
		}
	}
	if lastUpdate != nil && offset != nil {
		Fatalf(ds.Name() + ": you cannot use both date from and offset from\n")
	}
	if ctx.Category != "" {
		_, ok := ds.Categories()[ctx.Category]
		if !ok {
			Fatalf(ds.Name() + ": category " + ctx.Category + " not supported")
		}
	}
	err = ds.FetchItems(ctx)
	return
}

// Enrich - implement fetch raw data (generic)
func Enrich(ctx *Ctx, ds DS) (err error) {
	err = HandleMapping(ctx, ds, false)
	if err != nil {
		Fatalf(ds.Name()+": HandleMapping error: %+v\n", err)
	}
	if ds.CustomEnrich() {
		return ds.Enrich(ctx)
	}
	dbConfigured := ctx.AffsDBConfigured()
	if !dbConfigured && ctx.OnlyIdentities {
		Fatalf("Only identities mode specified and DB not configured")
	}
	if dbConfigured {
		ConnectAffiliationsDB(ctx)
	}
	var (
		lastUpdate *time.Time
		offset     *float64
		adjusted   bool
	)
	if ds.SupportDateFrom() {
		if ctx.DateFromDetected {
			lastUpdate = GetLastUpdate(ctx, ds, false)
			if lastUpdate != nil && (*lastUpdate).After(*ctx.DateFrom) {
				lastUpdate = ctx.DateFrom
				adjusted = true
			}
		} else {
			lastUpdate = ctx.DateFrom
		}
		if lastUpdate != nil {
			Printf("%s: rich: starting from date: %v, detected: %v, adjusted: %v\n", ds.Name(), *lastUpdate, ctx.DateFromDetected, adjusted)
		} else {
			Printf("%s: rich: no start date detected\n", ds.Name())
		}
		ctx.DateFrom = lastUpdate
	}
	if ds.SupportOffsetFrom() {
		adjusted = false
		if ctx.OffsetFromDetected {
			lastOffset := GetLastOffset(ctx, ds, false)
			if lastOffset >= 0.0 {
				offset = &lastOffset
				if lastOffset > ctx.OffsetFrom {
					offset = &ctx.OffsetFrom
					adjusted = true
				}
			}
		} else {
			if ctx.OffsetFrom >= 0.0 {
				offset = &ctx.OffsetFrom
			}
		}
		if offset != nil {
			Printf("%s: rich: starting from offset: %v, detected: %v, adjusted: %v\n", ds.Name(), *offset, ctx.OffsetFromDetected, adjusted)
			ctx.OffsetFrom = *offset
		} else {
			Printf("%s: rich: no start offset detected\n", ds.Name())
			ctx.OffsetFrom = -1.0
		}
	}
	if ctx.RefreshAffs {
		Printf("STUB: refresh affiliations\n")
		return
	}
	if ctx.AffsDBConfigured() {
		err = UploadIdentities(ctx, ds)
		if err != nil {
			Fatalf(ds.Name()+": UploadIdentities error: %+v\n", err)
		}
	}
	if ctx.OnlyIdentities {
		return
	}
	err = EnrichItems(ctx, ds)
	if err != nil {
		Fatalf(ds.Name()+": EnrichItems error: %+v\n", err)
	}
	return
}
