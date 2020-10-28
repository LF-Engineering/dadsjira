package dads

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// GerritBackendVersion - backend version
	GerritBackendVersion = "0.0.0"
	// GerritDefaultSSHKeyPath - default path to look for gerrit ssh private key
	GerritDefaultSSHKeyPath = "$HOME/.ssh/id_rsa"
	// GerritDefaultSSHPort - default gerrit ssh port
	GerritDefaultSSHPort = 29418
	// GerritDefaultMaxReviews = default max reviews when processing gerrit
	GerritDefaultMaxReviews = 500
)

var (
	// GerritRawMapping - Gerrit raw index mapping
	GerritRawMapping = []byte(`{"dynamic":true,"properties":{"metadata__updated_on":{"type":"date"},"data":{"properties":{"commitMessage":{"type":"text","index":true},"comments":{"properties":{"message":{"type":"text","index":true}}},"subject":{"type":"text","index":true},"patchSets":{"properties":{"approvals":{"properties":{"description":{"type":"text","index":true}}},"comments":{"properties":{"message":{"type":"text","index":true}}}}}}}}}`)
	// GerritRichMapping - Gerrit rich index mapping
	GerritRichMapping = []byte(`{"properties":{"metadata__updated_on":{"type":"date"},"approval_description_analyzed":{"type":"text","index":true},"comment_message_analyzed":{"type":"text","index":true},"status":{"type":"keyword"},"summary_analyzed":{"type":"text","index":true},"timeopen":{"type":"double"}}}`)
	// GerritCategories - categories defined for gerrit
	GerritCategories = map[string]struct{}{Review: {}}
	// GerritVersionRegexp - gerrit verion pattern
	GerritVersionRegexp = regexp.MustCompile(`gerrit version (\d+)\.(\d+).*`)
)

// DSGerrit - DS implementation for stub - does nothing at all, just presents a skeleton code
type DSGerrit struct {
	DS                  string
	URL                 string // From DA_GERRIT_URL - gerrit repo path
	SingleOrigin        bool   // From DA_GERRIT_SINGLE_ORIGIN - if you want to store only one gerrit endpoint in the index
	User                string // From DA_GERRIT_USER - gerrit user name
	SSHKey              string // From DA_GERRIT_SSH_KEY - must contain full SSH private key - has higher priority than key path
	SSHKeyPath          string // From DA_GERRIT_SSH_KEY_PATH - path to SSH private key, default GerritDefaultSSHKeyPath '~/.ssh/id_rsa'
	SSHPort             int    // From DA_GERRIT_SSH_PORT, defaults to GerritDefaultSSHPort (29418)
	MaxReviews          int    // From DA_GERRIT_MAX_REVIEWS, defaults to GerritDefaultMaxReviews (500)
	NoSSLVerify         bool   // From DA_GERRIT_NO_SSL_VERIFY
	DisableHostKeyCheck bool   // From DA_GERRIT_DISABLE_HOST_KEY_CHECK
	// Non-config variables
	RepoName       string   // repo name
	SSHOpts        string   // SSH Options
	SSHKeyTempPath string   // if used SSHKey - temp file with this name was used to store key contents
	GerritCmd      []string // gerrit remote command used to fetch data
	VersionMajor   int      // gerrit major version
	VersionMinor   int      // gerrit minor version
}

// ParseArgs - parse stub specific environment variables
func (j *DSGerrit) ParseArgs(ctx *Ctx) (err error) {
	j.DS = Gerrit
	prefix := "DA_GERRIT_"
	j.URL = os.Getenv(prefix + "URL")
	j.User = os.Getenv(prefix + "USER")
	j.SingleOrigin = StringToBool(os.Getenv(prefix + "SINGLE_ORIGIN"))
	if os.Getenv(prefix+"SSH_KEY_PATH") != "" {
		j.SSHKeyPath = os.Getenv(prefix + "SSH_KEY_PATH")
	} else {
		j.SSHKeyPath = GerritDefaultSSHKeyPath
	}
	j.SSHKey = os.Getenv(prefix + "SSH_KEY")
	j.NoSSLVerify = StringToBool(os.Getenv(prefix + "NO_SSL_VERIFY"))
	if j.NoSSLVerify {
		NoSSLVerify()
	}
	j.DisableHostKeyCheck = StringToBool(os.Getenv(prefix + "DISABLE_HOST_KEY_CHECK"))
	if ctx.Env("SSH_PORT") != "" {
		sshPort, err := strconv.Atoi(ctx.Env("SSH_PORT"))
		FatalOnError(err)
		if sshPort > 0 {
			j.SSHPort = sshPort
		}
	} else {
		j.SSHPort = GerritDefaultSSHPort
	}
	if ctx.Env("MAX_REVIEWS") != "" {
		maxReviews, err := strconv.Atoi(ctx.Env("MAX_REVIEWS"))
		FatalOnError(err)
		if maxReviews > 0 {
			j.MaxReviews = maxReviews
		}
	} else {
		j.MaxReviews = GerritDefaultMaxReviews
	}
	return
}

// Validate - is current DS configuration OK?
func (j *DSGerrit) Validate() (err error) {
	url := strings.TrimSpace(j.URL)
	if strings.HasSuffix(url, "/") {
		url = url[:len(url)-1]
	}
	ary := strings.Split(url, "/")
	j.RepoName = ary[len(ary)-1]
	if j.RepoName == "" {
		err = fmt.Errorf("Repo name must be set")
		return
	}
	j.SSHKeyPath = os.ExpandEnv(j.SSHKeyPath)
	if j.SSHKeyPath == "" && j.SSHKey == "" {
		err = fmt.Errorf("Either SSH key or SSH key path must be set")
		return
	}
	if j.User == "" {
		err = fmt.Errorf("User must be set")
	}
	return
}

// Name - return data source name
func (j *DSGerrit) Name() string {
	return j.DS
}

// Info - return DS configuration in a human readable form
func (j DSGerrit) Info() string {
	return fmt.Sprintf("%+v", j)
}

// CustomFetchRaw - is this datasource using custom fetch raw implementation?
func (j *DSGerrit) CustomFetchRaw() bool {
	return false
}

// FetchRaw - implement fetch raw data for stub datasource
func (j *DSGerrit) FetchRaw(ctx *Ctx) (err error) {
	Printf("%s should use generic FetchRaw()\n", j.DS)
	return
}

// CustomEnrich - is this datasource using custom enrich implementation?
func (j *DSGerrit) CustomEnrich() bool {
	return false
}

// Enrich - implement enrich data for stub datasource
func (j *DSGerrit) Enrich(ctx *Ctx) (err error) {
	Printf("%s should use generic Enrich()\n", j.DS)
	return
}

// InitGerrit - initializes gerrit client
func (j *DSGerrit) InitGerrit(ctx *Ctx) (err error) {
	if j.DisableHostKeyCheck {
		j.SSHOpts += "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null "
	}
	if j.SSHKey != "" {
		var f *os.File
		f, err = ioutil.TempFile("", "id_rsa")
		if err != nil {
			return
		}
		j.SSHKeyTempPath = f.Name()
		_, err = f.Write([]byte(j.SSHKey))
		if err != nil {
			return
		}
		err = f.Close()
		if err != nil {
			return
		}
		err = os.Chmod(j.SSHKeyTempPath, 0600)
		if err != nil {
			return
		}
		j.SSHOpts += "-i " + j.SSHKeyTempPath + " "
	} else {
		if j.SSHKeyPath != "" {
			j.SSHOpts += "-i " + j.SSHKeyPath + " "
		}
	}
	if strings.HasSuffix(j.SSHOpts, " ") {
		j.SSHOpts = j.SSHOpts[:len(j.SSHOpts)-1]
	}
	url := j.URL
	ary := strings.Split(url, "://")
	if len(ary) > 1 {
		url = ary[1]
	}
	gerritCmd := fmt.Sprintf("ssh %s -p %d %s@%s gerrit", j.SSHOpts, j.SSHPort, j.User, url)
	ary = strings.Split(gerritCmd, " ")
	for _, item := range ary {
		if item == "" {
			continue
		}
		j.GerritCmd = append(j.GerritCmd, item)
	}
	return
}

// GetGerritVersion - get gerrit version
func (j *DSGerrit) GetGerritVersion(ctx *Ctx) (err error) {
	cmdLine := j.GerritCmd
	cmdLine = append(cmdLine, "version")
	var (
		sout string
		serr string
	)
	sout, serr, err = ExecCommand(ctx, cmdLine, "", nil)
	if err != nil {
		Printf("error executing %v: %v\n%s\n%s\n", cmdLine, err, sout, serr)
		return
	}
	match := GerritVersionRegexp.FindAllStringSubmatch(sout, -1)
	if len(match) < 1 {
		err = fmt.Errorf("cannot parse gerrit version '%s'", sout)
		return
	}
	j.VersionMajor, _ = strconv.Atoi(match[0][1])
	j.VersionMinor, _ = strconv.Atoi(match[0][2])
	if ctx.Debug > 0 {
		Printf("Detected gerrit %d.%d\n", j.VersionMajor, j.VersionMinor)
	}
	return
}

// GetGerritReviews - get gerrit reviews
func (j *DSGerrit) GetGerritReviews(ctx *Ctx, after string, startFrom int) (err error) {
	cmdLine := j.GerritCmd
	// https://gerrit-review.googlesource.com/Documentation/user-search.html:
	// ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i ./ssh-key.secret -p XYZ usr@gerrit-url gerrit query after:1970-01-01 limit: 2 (status:open OR status:closed) --all-approvals --all-reviewers --comments --format=JSON
	cmdLine = append(cmdLine, "query")
	if ctx.Project != "" {
		cmdLine = append(cmdLine, "project:", ctx.Project)
	}
	cmdLine = append(cmdLine, "after:"+after, "limit:", strconv.Itoa(j.MaxReviews), "(status:open OR status:closed)", "--all-approvals", "--all-reviewers", "--comments", "--format=JSON")
	// 2006-01-02[ 15:04:05[.890][ -0700]]
	if startFrom > 0 {
		cmdLine = append(cmdLine, "--start="+strconv.Itoa(startFrom))
	}
	var (
		sout string
		serr string
	)
	if ctx.Debug > 0 {
		Printf("getting reviews via: %v\n", cmdLine)
	}
	sout, serr, err = ExecCommand(ctx, cmdLine, "", nil)
	if err != nil {
		Printf("error executing %v: %v\n%s\n%s\n", cmdLine, err, sout, serr)
		return
	}
	Printf("%s\n", sout)
	return
}

// FetchItems - implement enrich data for stub datasource
func (j *DSGerrit) FetchItems(ctx *Ctx) (err error) {
	err = j.InitGerrit(ctx)
	if err != nil {
		return
	}
	if j.SSHKeyTempPath != "" {
		defer func() {
			Printf("removing temporary SSH key %s\n", j.SSHKeyTempPath)
			//_ = os.Remove(j.SSHKeyTempPath)
		}()
	}
	err = j.GetGerritVersion(ctx)
	if err != nil {
		return
	}
	var after string
	if ctx.DateFrom != nil {
		after = ToYMDHMSDate(*ctx.DateFrom)
	} else {
		after = "1970-01-01"
	}
	startFrom := 0
	err = j.GetGerritReviews(ctx, after, startFrom)
	if err != nil {
		return
	}
	// FIXME
	os.Exit(1)
	var messages [][]byte
	// Process messages (possibly in threads)
	var (
		ch         chan error
		allMsgs    []interface{}
		allMsgsMtx *sync.Mutex
		escha      []chan error
		eschaMtx   *sync.Mutex
	)
	thrN := GetThreadsNum(ctx)
	if thrN > 1 {
		ch = make(chan error)
		allMsgsMtx = &sync.Mutex{}
		eschaMtx = &sync.Mutex{}
	}
	nThreads := 0
	processMsg := func(c chan error, msg []byte) (wch chan error, e error) {
		defer func() {
			if c != nil {
				c <- e
			}
		}()
		// FIXME: Real data processing here
		item := map[string]interface{}{"id": time.Now().UnixNano(), "name": "xyz"}
		esItem := j.AddMetadata(ctx, item)
		if ctx.Project != "" {
			item["project"] = ctx.Project
		}
		esItem["data"] = item
		if allMsgsMtx != nil {
			allMsgsMtx.Lock()
		}
		allMsgs = append(allMsgs, esItem)
		nMsgs := len(allMsgs)
		if nMsgs >= ctx.ESBulkSize {
			sendToElastic := func(c chan error) (ee error) {
				defer func() {
					if c != nil {
						c <- ee
					}
				}()
				ee = SendToElastic(ctx, j, true, UUID, allMsgs)
				if ee != nil {
					Printf("error %v sending %d messages to ElasticSearch\n", ee, len(allMsgs))
				}
				allMsgs = []interface{}{}
				if allMsgsMtx != nil {
					allMsgsMtx.Unlock()
				}
				return
			}
			if thrN > 1 {
				wch = make(chan error)
				go func() {
					_ = sendToElastic(wch)
				}()
			} else {
				e = sendToElastic(nil)
				if e != nil {
					return
				}
			}
		} else {
			if allMsgsMtx != nil {
				allMsgsMtx.Unlock()
			}
		}
		return
	}
	if thrN > 1 {
		for _, message := range messages {
			go func(msg []byte) {
				var (
					e    error
					esch chan error
				)
				esch, e = processMsg(ch, msg)
				if e != nil {
					Printf("process error: %v\n", e)
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
			}(message)
			nThreads++
			if nThreads == thrN {
				err = <-ch
				if err != nil {
					return
				}
				nThreads--
			}
		}
		for nThreads > 0 {
			err = <-ch
			nThreads--
			if err != nil {
				return
			}
		}
	} else {
		for _, message := range messages {
			_, err = processMsg(nil, message)
			if err != nil {
				return
			}
		}
	}
	for _, esch := range escha {
		err = <-esch
		if err != nil {
			return
		}
	}
	nMsgs := len(allMsgs)
	if ctx.Debug > 0 {
		Printf("%d remaining messages to send to ES\n", nMsgs)
	}
	if nMsgs > 0 {
		err = SendToElastic(ctx, j, true, UUID, allMsgs)
		if err != nil {
			Printf("Error %v sending %d messages to ES\n", err, len(allMsgs))
		}
	}
	return
}

// SupportDateFrom - does DS support resuming from date?
func (j *DSGerrit) SupportDateFrom() bool {
	return true
}

// SupportOffsetFrom - does DS support resuming from offset?
func (j *DSGerrit) SupportOffsetFrom() bool {
	return false
}

// DateField - return date field used to detect where to restart from
func (j *DSGerrit) DateField(*Ctx) string {
	return DefaultDateField
}

// RichIDField - return rich ID field name
func (j *DSGerrit) RichIDField(*Ctx) string {
	// IMPL:
	return DefaultIDField
}

// RichAuthorField - return rich ID field name
func (j *DSGerrit) RichAuthorField(*Ctx) string {
	return DefaultAuthorField
}

// OffsetField - return offset field used to detect where to restart from
func (j *DSGerrit) OffsetField(*Ctx) string {
	return DefaultOffsetField
}

// OriginField - return origin field used to detect where to restart from
func (j *DSGerrit) OriginField(ctx *Ctx) string {
	if ctx.Tag != "" {
		return DefaultTagField
	}
	// FIXME: number?
	return DefaultOriginField
}

// Categories - return a set of configured categories
func (j *DSGerrit) Categories() map[string]struct{} {
	return GerritCategories
}

// ResumeNeedsOrigin - is origin field needed when resuming
// Origin should be needed when multiple configurations save to the same index
func (j *DSGerrit) ResumeNeedsOrigin(ctx *Ctx) bool {
	return !j.SingleOrigin
}

// Origin - return current origin
func (j *DSGerrit) Origin(ctx *Ctx) string {
	// IMPL: you must change this, for example to j.URL/j.GroupName or somethign like this
	return ctx.Tag
}

// ItemID - return unique identifier for an item
func (j *DSGerrit) ItemID(item interface{}) string {
	// IMPL:
	// "number" ?
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// AddMetadata - add metadata to the item
func (j *DSGerrit) AddMetadata(ctx *Ctx, item interface{}) (mItem map[string]interface{}) {
	// IMPL:
	mItem = make(map[string]interface{})
	origin := "gerrit"
	tag := ctx.Tag
	if tag == "" {
		tag = origin
	}
	itemID := j.ItemID(item)
	updatedOn := j.ItemUpdatedOn(item)
	uuid := UUIDNonEmpty(ctx, origin, itemID)
	timestamp := time.Now()
	mItem["backend_name"] = j.DS
	mItem["backend_version"] = GerritBackendVersion
	mItem["timestamp"] = fmt.Sprintf("%.06f", float64(timestamp.UnixNano())/1.0e3)
	mItem[UUID] = uuid
	// FIXME: number?
	mItem[DefaultOriginField] = origin
	mItem[DefaultTagField] = tag
	mItem["updated_on"] = updatedOn
	mItem["category"] = j.ItemCategory(item)
	//mItem["search_fields"] = j.GenSearchFields(ctx, issue, uuid)
	//mItem["search_fields"] = make(map[string]interface{})
	mItem[DefaultDateField] = ToESDate(updatedOn)
	mItem[DefaultTimestampField] = ToESDate(timestamp)
	mItem[ProjectSlug] = ctx.ProjectSlug
	return
}

// ItemUpdatedOn - return updated on date for an item
func (j *DSGerrit) ItemUpdatedOn(item interface{}) time.Time {
	// IMPL:
	// "lastUpdated" ?
	return time.Now()
}

// ItemCategory - return unique identifier for an item
func (j *DSGerrit) ItemCategory(item interface{}) string {
	return Review
}

// ElasticRawMapping - Raw index mapping definition
func (j *DSGerrit) ElasticRawMapping() []byte {
	return GerritRawMapping
}

// ElasticRichMapping - Rich index mapping definition
func (j *DSGerrit) ElasticRichMapping() []byte {
	return GerritRichMapping
}

// GetItemIdentities return list of item's identities, each one is [3]string
// (name, username, email) tripples, special value Nil "<nil>" means null
// we use string and not *string which allows nil to allow usage as a map key
func (j *DSGerrit) GetItemIdentities(ctx *Ctx, doc interface{}) (map[[3]string]struct{}, error) {
	// IMPL:
	return map[[3]string]struct{}{}, nil
}

// GerritEnrichItemsFunc - iterate items and enrich them
// items is a current pack of input items
// docs is a pointer to where extracted identities will be stored
func GerritEnrichItemsFunc(ctx *Ctx, ds DS, thrN int, items []interface{}, docs *[]interface{}) (err error) {
	// IMPL:
	if ctx.Debug > 0 {
		Printf("gerrit enrich items %d/%d func\n", len(items), len(*docs))
	}
	var (
		mtx *sync.RWMutex
		ch  chan error
	)
	if thrN > 1 {
		mtx = &sync.RWMutex{}
		ch = make(chan error)
	}
	dbConfigured := ctx.AffsDBConfigured()
	nThreads := 0
	procItem := func(c chan error, idx int) (e error) {
		if thrN > 1 {
			mtx.RLock()
		}
		item := items[idx]
		if thrN > 1 {
			mtx.RUnlock()
		}
		defer func() {
			if c != nil {
				c <- e
			}
		}()
		src, ok := item.(map[string]interface{})["_source"]
		if !ok {
			e = fmt.Errorf("Missing _source in item %+v", DumpKeys(item))
			return
		}
		doc, ok := src.(map[string]interface{})
		if !ok {
			e = fmt.Errorf("Failed to parse document %+v\n", doc)
			return
		}
		if 1 == 0 {
			Printf("%v\n", dbConfigured)
		}
		// Actual item enrichment
		/*
			    var rich map[string]interface{}
					if thrN > 1 {
						mtx.Lock()
					}
					*docs = append(*docs, rich)
					if thrN > 1 {
						mtx.Unlock()
					}
		*/
		return
	}
	if thrN > 1 {
		for i := range items {
			go func(i int) {
				_ = procItem(ch, i)
			}(i)
			nThreads++
			if nThreads == thrN {
				err = <-ch
				if err != nil {
					return
				}
				nThreads--
			}
		}
		for nThreads > 0 {
			err = <-ch
			nThreads--
			if err != nil {
				return
			}
		}
		return
	}
	for i := range items {
		err = procItem(nil, i)
		if err != nil {
			return
		}
	}
	return
}

// EnrichItems - perform the enrichment
func (j *DSGerrit) EnrichItems(ctx *Ctx) (err error) {
	Printf("enriching items\n")
	err = ForEachESItem(ctx, j, true, ESBulkUploadFunc, GerritEnrichItemsFunc, nil)
	return
}

// EnrichItem - return rich item from raw item for a given author type
func (j *DSGerrit) EnrichItem(ctx *Ctx, item map[string]interface{}, author string, affs bool, extra interface{}) (rich map[string]interface{}, err error) {
	// IMPL:
	rich = item
	return
}

// AffsItems - return affiliations data items for given roles and date
func (j *DSGerrit) AffsItems(ctx *Ctx, rawItem map[string]interface{}, roles []string, date interface{}) (affsItems map[string]interface{}, err error) {
	// IMPL:
	return
}

// GetRoleIdentity - return identity data for a given role
func (j *DSGerrit) GetRoleIdentity(ctx *Ctx, item map[string]interface{}, role string) map[string]interface{} {
	// IMPL:
	return map[string]interface{}{"name": nil, "username": nil, "email": nil}
}

// AllRoles - return all roles defined for the backend
// roles can be static (always the same) or dynamic (per item)
// second return parameter is static mode (true/false)
// dynamic roles will use item to get its roles
func (j *DSGerrit) AllRoles(ctx *Ctx, item map[string]interface{}) ([]string, bool) {
	// IMPL:
	return []string{Author}, true
}
