package dads

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	jsoniter "github.com/json-iterator/go"
)

const (
	// JiraAPIRoot - main API path
	JiraAPIRoot = "/rest/api/2"
	// JiraAPISearch - search API subpath
	JiraAPISearch = "/search"
	// JiraAPIField - field API subpath
	JiraAPIField = "/field"
	// JiraAPIIssue - issue API subpath
	JiraAPIIssue = "/issue"
	// JiraAPIComment - comments API subpath
	JiraAPIComment = "/comment"
	// JiraBackendVersion - backend version
	JiraBackendVersion = "0.0.1"
	// JiraDefaultSearchField - default search field
	JiraDefaultSearchField = "item_id"
	// JiraDropCustomFields - drop custom fields from raw index
	JiraDropCustomFields = false
	// JiraFilterByProjectInComments - filter by project when searching for comments
	JiraFilterByProjectInComments = false
	// JiraMapCustomFields - run custom fields mapping
	JiraMapCustomFields = true
	// ClosedStatusCategoryKey - issue closed status key
	ClosedStatusCategoryKey = "done"
)

var (
	// JiraSearchFields - extra search fields
	JiraSearchFields = map[string][]string{
		"project_id":   {"fields", "project", "id"},
		"project_key":  {"fields", "project", "key"},
		"project_name": {"fields", "project", "name"},
		"issue_key":    {"key"},
	}
	// JiraRawMapping - Jira index mapping
	JiraRawMapping = []byte(`{"dynamic":true,"properties":{"metadata__updated_on":{"type":"date"},"data":{"properties":{"renderedFields":{"dynamic":false,"properties":{}},"operations":{"dynamic":false,"properties":{}},"fields":{"dynamic":true,"properties":{"description":{"type":"text","index":true},"environment":{"type":"text","index":true}}},"changelog":{"properties":{"histories":{"dynamic":false,"properties":{}}}},"comments_data":{"properties":{"body":{"type":"text","index":true}}}}}}}`)
	// JiraRichMapping - Jira index mapping
	JiraRichMapping = []byte(`{"properties":{"main_description_analyzed":{"type":"text","index":true},"releases":{"type":"keyword"},"body":{"type":"text","index":true}}}`)
)

// DSJira - DS implementation for Jira
type DSJira struct {
	DS          string
	URL         string // From DA_JIRA_URL - Jira URL
	NoSSLVerify bool   // From DA_JIRA_NO_SSL_VERIFY
	Token       string // From DA_JIRA_TOKEN
	PageSize    int    // From DA_JIRA_PAGE_SIZE
	MultiOrigin bool   // FROM DA_JIRA_MULTI_ORIGIN
}

// JiraField - informatin about fields present in issues
type JiraField struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Custom bool   `json:"custom"`
}

// ParseArgs - parse jira specific environment variables
func (j *DSJira) ParseArgs(ctx *Ctx) (err error) {
	j.DS = Jira
	// Jira specific env variables
	j.URL = os.Getenv("DA_JIRA_URL")
	j.NoSSLVerify = os.Getenv("DA_JIRA_NO_SSL_VERIFY") != ""
	j.Token = os.Getenv("DA_JIRA_TOKEN")
	if os.Getenv("DA_JIRA_PAGE_SIZE") == "" {
		j.PageSize = 500
	} else {
		pageSize, err := strconv.Atoi(os.Getenv("DA_JIRA_PAGE_SIZE"))
		FatalOnError(err)
		if pageSize > 0 {
			j.PageSize = pageSize
		}
	}
	j.MultiOrigin = os.Getenv("DA_JIRA_MULTI_ORIGIN") != ""
	if j.NoSSLVerify {
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return
}

// Validate - is current DS configuration OK?
func (j *DSJira) Validate() (err error) {
	if strings.HasSuffix(j.URL, "/") {
		j.URL = j.URL[:len(j.URL)-1]
	}
	if j.URL == "" {
		err = fmt.Errorf("Jira URL must be set")
	}
	return
}

// Name - return data source name
func (j *DSJira) Name() string {
	return j.DS
}

// Info - return DS configuration in a human readable form
func (j DSJira) Info() string {
	return fmt.Sprintf("%+v", j)
}

// CustomFetchRaw - is this datasource using custom fetch raw implementation?
func (j *DSJira) CustomFetchRaw() bool {
	return false
}

// FetchRaw - implement fetch raw data for Jira
func (j *DSJira) FetchRaw(ctx *Ctx) (err error) {
	Printf("%s should use generic FetchRaw()\n", j.DS)
	return
}

// CustomEnrich - is this datasource using custom enrich implementation?
func (j *DSJira) CustomEnrich() bool {
	return false
}

// Enrich - implement enrich data for Jira
func (j *DSJira) Enrich(ctx *Ctx) (err error) {
	Printf("%s should use generic Enrich()\n", j.DS)
	return
}

// GetFields - implement get fields for jira datasource
func (j *DSJira) GetFields(ctx *Ctx) (customFields map[string]JiraField, err error) {
	url := j.URL + JiraAPIRoot + JiraAPIField
	method := Get
	var headers map[string]string
	if j.Token != "" {
		headers = map[string]string{"Authorization": "Basic " + j.Token}
	}
	var resp interface{}
	resp, _, err = Request(ctx, url, method, headers, nil, nil, nil, map[[2]int]struct{}{{200, 200}: {}}, true)
	if err != nil {
		return
	}
	var fields []JiraField
	err = jsoniter.Unmarshal(resp.([]byte), &fields)
	if err != nil {
		return
	}
	customFields = make(map[string]JiraField)
	for _, field := range fields {
		if !field.Custom {
			continue
		}
		customFields[field.ID] = field
	}
	return
}

// GenSearchFields - generate extra search fields
func (j *DSJira) GenSearchFields(ctx *Ctx, issue interface{}, uuid string) (fields map[string]interface{}) {
	searchFields := j.SearchFields()
	fields = make(map[string]interface{})
	fields[JiraDefaultSearchField] = uuid
	for field, keyAry := range searchFields {
		item, _ := issue.(map[string]interface{})
		var value interface{}
		last := len(keyAry) - 1
		miss := false
		for i, key := range keyAry {
			var ok bool
			if i < last {
				item, ok = item[key].(map[string]interface{})
			} else {
				value, ok = item[key]
			}
			if !ok {
				Printf("%s: %+v, current: %s, %d/%d failed\n", field, keyAry, key, i+1, last+1)
				miss = true
				break
			}
		}
		if !miss {
			if ctx.Debug > 1 {
				Printf("Found %s: %+v --> %+v\n", field, keyAry, value)
			}
			fields[field] = value
		}
	}
	if ctx.Debug > 1 {
		Printf("Returing %+v\n", fields)
	}
	return
}

// ProcessIssue - process a single issue
func (j *DSJira) ProcessIssue(ctx *Ctx, allIssues *[]interface{}, allIssuesMtx *sync.Mutex, issue interface{}, customFields map[string]JiraField, from time.Time, to *time.Time, thrN int) (wch chan error, err error) {
	var mtx *sync.RWMutex
	if thrN > 1 {
		mtx = &sync.RWMutex{}
	}
	issueID := j.ItemID(issue)
	var headers map[string]string
	if j.Token != "" {
		headers = map[string]string{"Content-Type": "application/json", "Authorization": "Basic " + j.Token}
	} else {
		headers = map[string]string{"Content-Type": "application/json"}
	}
	processIssue := func(c chan error) (e error) {
		defer func() {
			if c != nil {
				c <- e
			}
		}()
		url := j.URL + JiraAPIRoot + JiraAPIIssue + "/" + issueID + JiraAPIComment
		startAt := int64(0)
		maxResults := int64(j.PageSize)
		epochMS := from.UnixNano() / 1e6
		// Seems like original Jira was using project filter there which is not needed IMHO.
		var jql string
		if JiraFilterByProjectInComments {
			if to != nil {
				epochToMS := (*to).UnixNano() / 1e6
				if ctx.Project != "" {
					jql = fmt.Sprintf(`"jql":"project = %s AND updated > %d AND updated < %d order by updated asc"`, ctx.Project, epochMS, epochToMS)
				} else {
					jql = fmt.Sprintf(`"jql":"updated > %d AND updated < %d order by updated asc"`, epochMS, epochToMS)
				}
			} else {
				if ctx.Project != "" {
					jql = fmt.Sprintf(`"jql":"project = %s AND updated > %d order by updated asc"`, ctx.Project, epochMS)
				} else {
					jql = fmt.Sprintf(`"jql":"updated > %d order by updated asc"`, epochMS)
				}
			}
		} else {
			if to != nil {
				epochToMS := (*to).UnixNano() / 1e6
				jql = fmt.Sprintf(`"jql":"updated > %d AND updated < %d order by updated asc"`, epochMS, epochToMS)
			} else {
				jql = fmt.Sprintf(`"jql":"updated > %d order by updated asc"`, epochMS)
			}
		}
		method := Get
		for {
			payloadBytes := []byte(fmt.Sprintf(`{"startAt":%d,"maxResults":%d,%s}`, startAt, maxResults, jql))
			var res interface{}
			res, _, e = Request(
				ctx,
				url,
				method,
				headers,
				payloadBytes,
				map[[2]int]struct{}{{200, 200}: {}}, // JSON statuses
				nil,                                 // Error statuses
				map[[2]int]struct{}{{200, 200}: {}}, // OK statuses: 200
				true,
			)
			if e != nil {
				return
			}
			comments, ok := res.(map[string]interface{})["comments"].([]interface{})
			if !ok {
				e = fmt.Errorf("unable to unmarshal comments from %+v", DumpKeys(res))
				return
			}
			if ctx.Debug > 1 {
				nComments := len(comments)
				if nComments > 0 {
					Printf("Processing %d comments\n", len(comments))
				}
			}
			if thrN > 1 {
				mtx.Lock()
			}
			issueComments, ok := issue.(map[string]interface{})["comments_data"].([]interface{})
			if !ok {
				issue.(map[string]interface{})["comments_data"] = []interface{}{}
			}
			issueComments, _ = issue.(map[string]interface{})["comments_data"].([]interface{})
			if !ok {
				issueComments = comments
			} else {
				issueComments = append(issueComments, comments...)
			}
			issue.(map[string]interface{})["comments_data"] = issueComments
			if thrN > 1 {
				mtx.Unlock()
			}
			totalF, ok := res.(map[string]interface{})["total"].(float64)
			if !ok {
				e = fmt.Errorf("unable to unmarshal total from %+v", DumpKeys(res))
				return
			}
			maxResultsF, ok := res.(map[string]interface{})["maxResults"].(float64)
			if !ok {
				e = fmt.Errorf("unable to maxResults total from %+v", DumpKeys(res))
				return
			}
			total := int64(totalF)
			maxResults = int64(maxResultsF)
			inc := int64(totalF)
			if maxResultsF < totalF {
				inc = int64(maxResultsF)
			}
			startAt += inc
			if startAt >= total {
				startAt = total
				break
			}
			if ctx.Debug > 0 {
				Printf("Processing next comments page from %d/%d\n", startAt, total)
			}
		}
		if ctx.Debug > 1 {
			Printf("Processed %d comments\n", startAt)
		}
		return
	}
	var ch chan error
	if thrN > 1 {
		ch = make(chan error)
		go func() {
			_ = processIssue(ch)
		}()
	} else {
		err = processIssue(nil)
		if err != nil {
			return
		}
	}
	if thrN > 1 {
		mtx.RLock()
	}
	issueFields, ok := issue.(map[string]interface{})["fields"].(map[string]interface{})
	if thrN > 1 {
		mtx.RUnlock()
	}
	if !ok {
		err = fmt.Errorf("unable to unmarshal fields from issue %+v", DumpKeys(issue))
		return
	}
	if JiraMapCustomFields {
		type mapping struct {
			ID    string
			Name  string
			Value interface{}
		}
		m := make(map[string]mapping)
		for k, v := range issueFields {
			customField, ok := customFields[k]
			if !ok {
				continue
			}
			m[k] = mapping{ID: customField.ID, Name: customField.Name, Value: v}
		}
		for k, v := range m {
			if ctx.Debug > 1 {
				prev := issueFields[k]
				Printf("%s: %+v -> %+v\n", k, prev, v)
			}
			issueFields[k] = v
		}
	}
	// Extra fields
	esItem := make(map[string]interface{})
	origin := j.URL
	tag := ctx.Tag
	if tag == "" {
		tag = origin
	}
	uuid := UUIDNonEmpty(ctx, origin, issueID)
	timestamp := time.Now()
	esItem["backend_name"] = j.DS
	esItem["backend_version"] = JiraBackendVersion
	esItem["timestamp"] = fmt.Sprintf("%.06f", float64(timestamp.UnixNano())/1.0e3)
	esItem[UUID] = uuid
	esItem[DefaultOriginField] = origin
	esItem[DefaultTagField] = tag
	if thrN > 1 {
		mtx.Lock()
	}
	updatedOn := j.ItemUpdatedOn(issue)
	esItem["updated_on"] = updatedOn
	esItem["category"] = j.ItemCategory(issue)
	esItem["search_fields"] = j.GenSearchFields(ctx, issue, uuid)
	// Seems like it doesn't make sense, because we just added those custom fields
	if JiraDropCustomFields {
		for k := range issueFields {
			if strings.HasPrefix(strings.ToLower(k), "customfield_") {
				delete(issueFields, k)
			}
		}
	}
	esItem[DefaultDateField] = ToESDate(updatedOn)
	esItem[DefaultTimestampField] = ToESDate(timestamp)
	if ctx.Project != "" {
		issue.(map[string]interface{})["project"] = ctx.Project
	}
	esItem["data"] = issue
	if thrN > 1 {
		mtx.Unlock()
		err = <-ch
	}
	if allIssuesMtx != nil {
		allIssuesMtx.Lock()
	}
	*allIssues = append(*allIssues, esItem)
	nIssues := len(*allIssues)
	if nIssues >= ctx.ESBulkSize {
		sendToElastic := func(c chan error) (e error) {
			defer func() {
				if c != nil {
					c <- e
				}
			}()
			e = SendToElastic(ctx, j, true, UUID, *allIssues)
			if e != nil {
				Printf("Error %v sending %d issues to ElasticSearch\n", e, len(*allIssues))
			}
			*allIssues = []interface{}{}
			if allIssuesMtx != nil {
				allIssuesMtx.Unlock()
			}
			return
		}
		if thrN > 1 {
			wch = make(chan error)
			go func() {
				_ = sendToElastic(wch)
			}()
		} else {
			err = sendToElastic(nil)
			if err != nil {
				return
			}
		}
	} else {
		if allIssuesMtx != nil {
			allIssuesMtx.Unlock()
		}
	}
	return
}

// FetchItems - implement fetch items for jira datasource
func (j *DSJira) FetchItems(ctx *Ctx) (err error) {
	thrN := GetThreadsNum(ctx)
	var customFields map[string]JiraField
	fieldsFetched := false
	var chF chan error
	getFields := func(c chan error) (e error) {
		defer func() {
			if c != nil {
				c <- e
			}
			if ctx.Debug > 0 {
				Printf("Got %d custom fields\n", len(customFields))
			}
		}()
		customFields, e = j.GetFields(ctx)
		return
	}
	if thrN > 1 {
		chF = make(chan error)
		go func() {
			_ = getFields(chF)
		}()
	} else {
		err = getFields(nil)
		if err != nil {
			Printf("GetFields error: %+v\n", err)
			return
		}
		fieldsFetched = true
	}
	// '{"jql":"updated > 1601281314000 order by updated asc","startAt":0,"maxResults":400,"expand":["renderedFields","transitions","operations","changelog"]}'
	var (
		from time.Time
		to   *time.Time
	)
	if ctx.DateFrom != nil {
		from = *ctx.DateFrom
	} else {
		from = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	to = ctx.DateTo
	url := j.URL + JiraAPIRoot + JiraAPISearch
	startAt := int64(0)
	maxResults := int64(j.PageSize)
	jql := ""
	epochMS := from.UnixNano() / 1e6
	if to != nil {
		epochToMS := (*to).UnixNano() / 1e6
		if ctx.Project != "" {
			jql = fmt.Sprintf(`"jql":"project = %s AND updated > %d AND updated < %d order by updated asc"`, ctx.Project, epochMS, epochToMS)
		} else {
			jql = fmt.Sprintf(`"jql":"updated > %d AND updated < %d order by updated asc"`, epochMS, epochToMS)
		}
	} else {
		if ctx.Project != "" {
			jql = fmt.Sprintf(`"jql":"project = %s AND updated > %d order by updated asc"`, ctx.Project, epochMS)
		} else {
			jql = fmt.Sprintf(`"jql":"updated > %d order by updated asc"`, epochMS)
		}
	}
	expand := `"expand":["renderedFields","transitions","operations","changelog"]`
	allIssues := []interface{}{}
	var allIssuesMtx *sync.Mutex
	var escha []chan error
	var eschaMtx *sync.Mutex
	var chE chan error
	if thrN > 1 {
		chE = make(chan error)
		allIssuesMtx = &sync.Mutex{}
		eschaMtx = &sync.Mutex{}
	}
	nThreads := 0
	method := Post
	var headers map[string]string
	if j.Token != "" {
		// Token should be BASE64("useremail:api_token"), see: https://developer.atlassian.com/cloud/jira/platform/basic-auth-for-rest-apis
		headers = map[string]string{"Content-Type": "application/json", "Authorization": "Basic " + j.Token}
	} else {
		headers = map[string]string{"Content-Type": "application/json"}
	}
	if ctx.Debug > 0 {
		Printf("requesting issues from: %s\n", from)
	}
	for {
		payloadBytes := []byte(fmt.Sprintf(`{"startAt":%d,"maxResults":%d,%s,%s}`, startAt, maxResults, jql, expand))
		var res interface{}
		res, _, err = Request(
			ctx,
			url,
			method,
			headers,
			payloadBytes,
			map[[2]int]struct{}{{200, 200}: {}}, // JSON statuses
			nil,                                 // Error statuses
			map[[2]int]struct{}{{200, 200}: {}}, // OK statuses: 200, 404
			true,
		)
		if err != nil {
			return
		}
		if !fieldsFetched {
			err = <-chF
			if err != nil {
				Printf("GetFields error: %+v\n", err)
				return
			}
			fieldsFetched = true
		}
		processIssues := func(c chan error) (e error) {
			defer func() {
				if c != nil {
					c <- e
				}
			}()
			issues, ok := res.(map[string]interface{})["issues"].([]interface{})
			if !ok {
				e = fmt.Errorf("unable to unmarshal issues from %+v", DumpKeys(res))
				return
			}
			if ctx.Debug > 0 {
				Printf("Processing %d issues\n", len(issues))
			}
			for _, issue := range issues {
				var esch chan error
				esch, e = j.ProcessIssue(ctx, &allIssues, allIssuesMtx, issue, customFields, from, to, thrN)
				if e != nil {
					Printf("Error %v processing issue: %+v\n", e, issue)
					return
				}
				if esch != nil {
					if eschaMtx != nil {
						eschaMtx.Lock()
					}
					escha = append(escha, esch)
					if eschaMtx != nil {
						eschaMtx.Unlock()
					}
				}
			}
			return
		}
		if thrN > 1 {
			go func() {
				_ = processIssues(chE)
			}()
			nThreads++
			if nThreads == thrN {
				err = <-chE
				if err != nil {
					return
				}
				nThreads--
			}
		} else {
			err = processIssues(nil)
			if err != nil {
				return
			}
		}
		totalF, ok := res.(map[string]interface{})["total"].(float64)
		if !ok {
			err = fmt.Errorf("unable to unmarshal total from %+v", DumpKeys(res))
			return
		}
		maxResultsF, ok := res.(map[string]interface{})["maxResults"].(float64)
		if !ok {
			err = fmt.Errorf("unable to maxResults total from %+v", DumpKeys(res))
			return
		}
		total := int64(totalF)
		maxResults = int64(maxResultsF)
		inc := int64(totalF)
		if maxResultsF < totalF {
			inc = int64(maxResultsF)
		}
		startAt += inc
		if startAt >= total {
			startAt = total
			break
		}
		if ctx.Debug > 0 {
			Printf("Processing next issues page from %d/%d\n", startAt, total)
		}
	}
	for thrN > 1 && nThreads > 0 {
		err = <-chE
		nThreads--
		if err != nil {
			return
		}
	}
	for _, esch := range escha {
		err = <-esch
		if err != nil {
			return
		}
	}
	nIssues := len(allIssues)
	if ctx.Debug > 0 {
		Printf("%d remaining issues to send to ElasticSearch\n", nIssues)
	}
	if nIssues > 0 {
		err = SendToElastic(ctx, j, true, UUID, allIssues)
		if err != nil {
			Printf("Error %v sending %d issues to ElasticSearch\n", err, len(allIssues))
		}
	}
	Printf("Processed %d issues\n", startAt)
	return
}

// SupportDateFrom - does DS support resuming from date?
func (j *DSJira) SupportDateFrom() bool {
	return true
}

// SupportOffsetFrom - does DS support resuming from offset?
func (j *DSJira) SupportOffsetFrom() bool {
	return false
}

// DateField - return date field used to detect where to restart from
func (j *DSJira) DateField(*Ctx) string {
	return DefaultDateField
}

// RichIDField - return rich ID field name
func (j *DSJira) RichIDField(*Ctx) string {
	return DefaultIDField
}

// OffsetField - return offset field used to detect where to restart from
func (j *DSJira) OffsetField(*Ctx) string {
	return DefaultOffsetField
}

//Categories - return a set of configured categories
func (j *DSJira) Categories() map[string]struct{} {
	return map[string]struct{}{"issue": {}}
}

// OriginField - return origin field used to detect where to restart from
func (j *DSJira) OriginField(ctx *Ctx) string {
	if ctx.Tag != "" {
		return DefaultTagField
	}
	return DefaultOriginField
}

// ResumeNeedsOrigin - is origin field needed when resuming
// Origin should be needed when multiple configurations save to the same index
// Jira usually stores only one instance per index, so we don't need to enable filtering by origin to resume
func (j *DSJira) ResumeNeedsOrigin(ctx *Ctx) bool {
	return j.MultiOrigin
}

// Origin - return current origin
// Tag gets precendence if set
func (j *DSJira) Origin(ctx *Ctx) string {
	if ctx.Tag != "" {
		return ctx.Tag
	}
	return j.URL
}

// ItemID - return unique identifier for an item
func (j *DSJira) ItemID(item interface{}) string {
	id, ok := item.(map[string]interface{})["id"].(string)
	if !ok {
		Fatalf("%s: ItemID() - cannot extract id from %+v", j.DS, item)
	}
	return id
}

// ItemUpdatedOn - return updated on date for an item
func (j *DSJira) ItemUpdatedOn(item interface{}) time.Time {
	fields, ok := item.(map[string]interface{})["fields"].(map[string]interface{})
	if !ok {
		Fatalf("%s: ItemUpdatedOn() - cannot extract fields from %+v", j.DS, item)
	}
	sUpdated, ok := fields["updated"].(string)
	if !ok {
		Fatalf("%s: ItemUpdatedOn() - cannot extract updated from %+v", j.DS, fields)
	}
	updated, err := TimeParseES(sUpdated)
	FatalOnError(err)
	return updated
}

// ItemCategory - return unique identifier for an item
func (j *DSJira) ItemCategory(item interface{}) string {
	return Issue
}

// SearchFields - define (optional) search fields to be returned
func (j *DSJira) SearchFields() map[string][]string {
	return JiraSearchFields
}

// ElasticRawMapping - Raw index mapping definition
func (j *DSJira) ElasticRawMapping() []byte {
	return JiraRawMapping
}

// ElasticRichMapping - Rich index mapping definition
func (j *DSJira) ElasticRichMapping() []byte {
	return JiraRichMapping
}

// GetItemIdentities return list of item's identities, each one is [3]string
// (name, username, email) tripples, special value Nil "<nil>" means null
// we use string and not *string which allows nil to allow usage as a map key
func (j *DSJira) GetItemIdentities(ctx *Ctx, doc interface{}) (identities map[[3]string]struct{}, err error) {
	fields, ok := doc.(map[string]interface{})["data"].(map[string]interface{})["fields"].(map[string]interface{})
	if !ok {
		err = fmt.Errorf("cannot read data.fields from doc %+v", DumpKeys(doc))
		return
	}
	init := false
	for _, field := range []string{"assignee", "reporter", "creator"} {
		f, ok := fields[field].(map[string]interface{})
		if !ok {
			// Printf("Field %s not found\n", field)
			continue
		}
		any := false
		identity := [3]string{}
		for i, k := range []string{"displayName", "name", "emailAddress"} {
			v, ok := f[k].(string)
			// Printf("%d: (%s,%s) -> (%s, %v)\n", i, field, k, v, ok)
			if ok {
				identity[i] = v
				any = true
			} else {
				identity[i] = Nil
			}
		}
		if any {
			if !init {
				identities = make(map[[3]string]struct{})
				init = true
			}
			identities[identity] = struct{}{}
		}
	}
	comments, ok := doc.(map[string]interface{})["data"].(map[string]interface{})["comments_data"].([]interface{})
	if !ok {
		err = fmt.Errorf("cannot read data.comments_data from doc %+v", DumpKeys(doc))
		return
	}
	for _, rawComment := range comments {
		comment, ok := rawComment.(map[string]interface{})
		if !ok {
			err = fmt.Errorf("Cannot parse %+v\n", rawComment)
			return
		}
		for _, field := range []string{"author", "updateAuthor"} {
			f, ok := comment[field].(map[string]interface{})
			if !ok {
				// Printf("Field %s not found\n", field)
				continue
			}
			any := false
			identity := [3]string{}
			for i, k := range []string{"displayName", "name", "emailAddress"} {
				v, ok := f[k].(string)
				// Printf("%d: (%s,%s) -> (%s, %v)\n", i, field, k, v, ok)
				if ok {
					identity[i] = v
					any = true
				} else {
					identity[i] = Nil
				}
			}
			if any {
				if !init {
					identities = make(map[[3]string]struct{})
					init = true
				}
				identities[identity] = struct{}{}
			}
		}
	}
	return
}

// JiraEnrichItemsFunc - iterate items and enrich them
// items is a current pack of input items
// docs is a pointer to where extracted identities will be stored
func JiraEnrichItemsFunc(ctx *Ctx, ds DS, items []interface{}, docs *[]interface{}) (err error) {
	dbConfigured := ctx.AffsDBConfigured()
	var rich map[string]interface{}
	for _, item := range items {
		src, ok := item.(map[string]interface{})["_source"]
		if !ok {
			err = fmt.Errorf("Missing _source in item %+v", DumpKeys(item))
			return
		}
		doc, ok := src.(map[string]interface{})
		if !ok {
			err = fmt.Errorf("Failed to parse document %+v\n", doc)
			return
		}
		for _, author := range []string{"creator", "assignee", "reporter"} {
			rich, err = ds.EnrichItem(ctx, doc, author, dbConfigured)
			if err != nil {
				return
			}
			*docs = append(*docs, rich)
		}
		// FIXME: enrich comments continue
	}
	// fmt.Printf("currently %d jira enriched docs\n", len(*docs))
	return
}

// EnrichItems - perform the enrichment
func (j *DSJira) EnrichItems(ctx *Ctx) (err error) {
	err = ForEachRawItem(ctx, j, ctx.ESBulkSize, ESBulkUploadFunc, JiraEnrichItemsFunc)
	return
}

// EnrichItem - return rich item from raw item for a given author type
func (j *DSJira) EnrichItem(ctx *Ctx, item map[string]interface{}, author string, affs bool) (rich map[string]interface{}, err error) {
	// copy RawFields
	rich = make(map[string]interface{})
	for _, field := range RawFields {
		v, ok := item[field]
		if !ok {
			continue
		}
		rich[field] = v
	}
	issue, ok := item["data"].(map[string]interface{})
	if !ok {
		err = fmt.Errorf("missing data field in item %+v", DumpKeys(item))
		return
	}
	changes, ok := Dig(issue, []string{"changelog", "total"}, false, false)
	if ok {
		rich["channges"] = changes
	} else {
		// Only evil Jiras do that, for example http://jira.akraino.org
		// Almost the same address works OK https://jira.akraino.org
		rich["channges"] = 0
	}
	fields, ok := issue["fields"].(map[string]interface{})
	if !ok {
		err = fmt.Errorf("missing fields field in issue %+v", DumpKeys(issue))
		return
	}
	for _, field := range []string{"assignee", "reporter"} {
		v, _ := issue[field]
		rich[field] = v
	}
	for _, field := range []string{"creator", "assignee", "reporter"} {
		v, ok := fields[field].(map[string]interface{})
		if !ok || v == nil {
			continue
		}
		tz, ok := v["timeZone"]
		if ok {
			rich[field+"_tz"] = tz
		}
		if field == "assignee" {
			name, _ := v["displayName"]
			rich[field] = name
		} else {
			name, _ := v["displayName"]
			login, _ := v["name"]
			rich[field+"_name"] = name
			rich[field+"_login"] = login
		}
	}
	authorName, _ := rich[author+"_name"]
	authorLogin, _ := rich[author+"_login"]
	authorTz, _ := rich[author+"_tz"]
	rich["author_type"] = author
	rich["author_name"] = authorName
	rich["author_login"] = authorLogin
	rich["author_tz"] = authorTz
	created, _ := Dig(fields, []string{"created"}, true, false)
	rich["creation_date"] = created
	desc, ok := fields["description"].(string)
	if ok {
		rich["main_description_analyzed"] = desc
		if len(desc) > KeywordMaxlength {
			desc = desc[:KeywordMaxlength]
		}
		rich["main_description"] = desc
	}
	rich["issue_type"], _ = Dig(fields, []string{"issuetype", "name"}, true, false)
	rich["issue_description"], _ = Dig(fields, []string{"issuetype", "description"}, true, false)
	labels, ok := fields["labels"]
	if ok {
		rich["labels"] = labels
	}
	priority, ok := Dig(fields, []string{"priority", "name"}, false, true)
	if ok {
		rich["priority"] = priority
	}
	progress, ok := Dig(fields, []string{"progress", "total"}, false, true)
	if ok {
		rich["progress_total"] = progress
	}
	rich["project_id"], _ = Dig(fields, []string{"project", "id"}, true, false)
	rich["project_key"], _ = Dig(fields, []string{"project", "key"}, true, false)
	rich["project_name"], _ = Dig(fields, []string{"project", "name"}, true, false)
	resolution, ok := fields["resolution"]
	if ok && resolution != nil {
		rich["resolution_id"], _ = Dig(resolution, []string{"id"}, true, false)
		rich["resolution_name"], _ = Dig(resolution, []string{"name"}, true, false)
		rich["resolution_description"], _ = Dig(resolution, []string{"description"}, true, false)
		rich["resolution_self"], _ = Dig(resolution, []string{"self"}, true, false)
	}
	rich["resolution_date"], _ = Dig(fields, []string{"resolutiondate"}, true, false)
	rich["status_description"], _ = Dig(fields, []string{"status", "description"}, true, false)
	rich["status"], _ = Dig(fields, []string{"status", "name"}, true, false)
	rich["status_category_key"], _ = Dig(fields, []string{"status", "statusCategory", "key"}, true, false)
	rich["is_closed"] = 0
	catKey, _ := rich["status_category_key"].(string)
	if catKey == ClosedStatusCategoryKey {
		rich["is_closed"] = 1
	}
	rich["summary"], _ = Dig(fields, []string{"summary"}, true, false)
	timeoriginalestimate, ok := Dig(fields, []string{"timeoriginalestimate"}, false, true)
	if ok {
		rich["original_time_estimation"] = timeoriginalestimate
		if timeoriginalestimate != nil {
			fVal, ok := timeoriginalestimate.(float64)
			if ok {
				rich["original_time_estimation_hours"] = int(fVal / 3600.0)
			}
		}
	}
	timespent, ok := Dig(fields, []string{"timespent"}, false, true)
	if ok {
		rich["time_spent"] = timespent
		if timespent != nil {
			fVal, ok := timespent.(float64)
			if ok {
				rich["time_spent_hours"] = int(fVal / 3600.0)
			}
		}
	}
	timeestimate, ok := Dig(fields, []string{"timeestimate"}, false, true)
	if ok {
		rich["time_estimation"] = timeestimate
		if timeestimate != nil {
			fVal, ok := timeestimate.(float64)
			if ok {
				rich["time_estimation_hours"] = int(fVal / 3600.0)
			}
		}
	}
	rich["watchers"], _ = Dig(fields, []string{"watches", "watchCount"}, true, false)
	iKey, _ := Dig(issue, []string{"key"}, true, false)
	key, ok := iKey.(string)
	if !ok {
		err = fmt.Errorf("cannot read key as string from %T %+v", iKey, iKey)
		return
	}
	rich["key"] = key
	iid, ok := issue["id"].(string)
	if !ok {
		err = fmt.Errorf("missing int id field in issue %+v", DumpKeys(issue))
		return
	}
	rich["id"] = fmt.Sprintf("%s_issue_%s_user_%s", rich[UUID], iid, author)
	rich["number_of_comments"] = 0
	comments, ok := issue["comments_data"].([]interface{})
	if ok {
		rich["number_of_comments"] = len(comments)
	}
	updated, _ := Dig(fields, []string{"updated"}, false, true)
	rich["updated"] = updated
	origin, ok := rich[DefaultOriginField].(string)
	if !ok {
		err = fmt.Errorf("cannot read origin as string from rich %+v", rich)
		return
	}
	rich["url"] = origin + "/browse/" + key
	var (
		sCreated  string
		createdDt time.Time
		sUpdated  string
		updatedDt time.Time
		e         error
		o         bool
	)
	sCreated, o = created.(string)
	if o {
		createdDt, e = TimeParseES(sCreated)
		if e != nil {
			o = false
		}
	}
	if o {
		sUpdated, o = updated.(string)
	}
	if o {
		updatedDt, e = TimeParseES(sUpdated)
		if e != nil {
			o = false
		}
	}
	if o {
		now := time.Now()
		days := float64(updatedDt.Sub(createdDt).Seconds()) / 86400.0
		rich["time_to_close_days"] = days
		days = float64(now.Sub(createdDt).Seconds()) / 86400.0
		rich["time_to_last_update_days"] = days
	} else {
		rich["time_to_close_days"] = nil
		rich["time_to_last_update_days"] = nil
	}
	fixVersions, ok := Dig(fields, []string{"fixVersions"}, false, true)
	if ok {
		rels := []interface{}{}
		versions, ok := fixVersions.([]interface{})
		if ok {
			for _, version := range versions {
				name, ok := Dig(version, []string{"name"}, false, true)
				if ok {
					rels = append(rels, name)
				}
			}
		}
		rich["releases"] = rels
	}
	for field, fieldValue := range fields {
		if !strings.HasPrefix(strings.ToLower(field), "customfield_") {
			continue
		}
		f, ok := fieldValue.(map[string]interface{})
		if !ok {
			continue
		}
		name, ok := f["Name"]
		if !ok {
			continue
		}
		if name == "Story Points" {
			rich["story_points"] = f["value"]
		} else if name == "Sprint" {
			v, ok := f["value"]
			if !ok {
				continue
			}
			iAry, ok := v.([]interface{})
			if !ok {
				continue
			}
			if len(iAry) == 0 {
				continue
			}
			s, ok := iAry[0].(string)
			if !ok {
				continue
			}
			rich["sprint"] = strings.Split(PartitionString(s, ",name=")[2], ",")[0]
			rich["sprint_start"] = strings.Split(PartitionString(s, ",startDate=")[2], ",")[0]
			rich["sprint_end"] = strings.Split(PartitionString(s, ",endDate=")[2], ",")[0]
			rich["sprint_complete"] = strings.Split(PartitionString(s, ",completeDate=")[2], ",")[0]
		}
	}
	// If affiliations DB enabled
	if affs {
		var affsItems map[string]interface{}
		affsItems, err = j.AffsItems(ctx, item, []string{"assignee", "reporter", "creator"}, created)
		if err != nil {
			return
		}
		for prop, value := range affsItems {
			rich[prop] = value
		}
		suffs := []string{"_id", "_uuid", "_name", "_user_name", "_domain", "_gender", "_gender_acc", "_org_name", "_bot"}
		for _, suff := range suffs {
			rich["author"+suff] = rich[author+suff]
		}
		orgsKey := author + MultiOrgNames
		_, ok := Dig(rich, []string{orgsKey}, false, true)
		if !ok {
			rich[orgsKey] = []interface{}{}
		}
	}
	for prop, value := range CommonFields(j, created, Issue) {
		rich[prop] = value
	}
	rich["type"] = Issue
	return
}

// AffsItems - return affiliations data items for given roles and date
func (j *DSJira) AffsItems(ctx *Ctx, item map[string]interface{}, roles []string, date interface{}) (affsItems map[string]interface{}, err error) {
	affsItems = make(map[string]interface{})
	var dt time.Time
	sDate, ok := date.(string)
	if !ok {
		err = fmt.Errorf("%+v %T is not a string", date, date)
		return
	}
	dt, err = TimeParseES(sDate)
	if err != nil {
		return
	}
	for _, role := range roles {
		identity := j.GetRoleIdentity(ctx, item, role)
		if len(identity) == 0 {
			continue
		}
		affsIdentity := IdenityAffsData(ctx, j, identity, dt, role)
		for prop, value := range affsIdentity {
			affsItems[prop] = value
		}
		suffs := []string{"_org_name", "_name", "_user_name"}
		for _, suff := range suffs {
			k := role + suff
			_, ok := affsIdentity[k]
			if !ok {
				affsIdentity[k] = Unknown
			}
		}
	}
	return
}

// GetRoleIdentity - return identity data for a given role
func (j *DSJira) GetRoleIdentity(ctx *Ctx, item map[string]interface{}, role string) (identity map[string]interface{}) {
	identity = make(map[string]interface{})
	fields, _ := Dig(item, []string{"data", "fields"}, true, false)
	user, ok := Dig(fields, []string{role}, false, true)
	if !ok {
		return
	}
	data := [][2]string{
		{"name", "displayName"},
		{"username", "name"},
		{"email", "emailAddress"},
	}
	for _, row := range data {
		v, _ := Dig(user, []string{row[1]}, false, true)
		identity[row[0]] = v
	}
	return
}
