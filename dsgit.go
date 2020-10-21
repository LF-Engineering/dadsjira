package dads

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	jsoniter "github.com/json-iterator/go"
)

const (
	// GitBackendVersion - backend version
	GitBackendVersion = "0.0.1"
	// GitDefaultReposPath - default path where git repository clones
	GitDefaultReposPath = "$HOME/.perceval/repositories"
	// GitDefaultCachePath - default path where gitops cache files are stored
	GitDefaultCachePath = "$HOME/.perceval/cache"
	// GitOpsCommand - command that maintains git stats cache
	GitOpsCommand = "gitops.py"
	// GitOpsNoCleanup - if set, it will skip gitops.py repo cleanup
	// FIXME: turn off when finshed
	GitOpsNoCleanup = true
)

var (
	// GitRawMapping - Git raw index mapping
	GitRawMapping = []byte(`{"dynamic":true,"properties":{"metadata__updated_on":{"type":"date"},"data":{"properties":{"message":{"type":"text","index":true}}}}}`)
	// GitRichMapping - Git rich index mapping
	GitRichMapping = []byte(`{"properties":{"metadata__updated_on":{"type":"date"},"message_analyzed":{"type":"text","index":true}}}`)
)

// RawPLS - programming language summary (all fields as strings)
type RawPLS struct {
	Language string `json:"language"`
	Files    string `json:"files"`
	Blank    string `json:"blank"`
	Comment  string `json:"comment"`
	Code     string `json:"code"`
}

// PLS - programming language summary
type PLS struct {
	Language string `json:"language"`
	Files    int    `json:"files"`
	Blank    int    `json:"blank"`
	Comment  int    `json:"comment"`
	Code     int    `json:"code"`
}

// DSGit - DS implementation for git - does nothing at all, just presents a skeleton code
type DSGit struct {
	DS           string
	URL          string // From DA_GIT_URL - git repo path
	SingleOrigin bool   // From DA_GIT_SINGLE_ORIGIN - if you want to store only one git endpoint in the index
	ReposPath    string // From DA_GIT_REPOS_PATH - default GitDefaultReposPath
	CachePath    string // From DA_GIT_CACHE_PATH - default GitDefaultCachePath
	NoSSLVerify  bool   // From DA_GIT_NO_SSL_VERIFY
	// Non-config variables
	RepoName string // repo name
	Loc      int    // lines of code as reported by GitOpsCommand
	Pls      []PLS  // programming language suppary as reported by GitOpsCommand
	GitPath  string // path to git repo clone
}

// ParseArgs - parse git specific environment variables
func (j *DSGit) ParseArgs(ctx *Ctx) (err error) {
	j.DS = Git
	prefix := "DA_GIT_"
	j.URL = os.Getenv(prefix + "URL")
	j.SingleOrigin = StringToBool(os.Getenv(prefix + "SINGLE_ORIGIN"))
	if os.Getenv(prefix+"REPOS_PATH") != "" {
		j.ReposPath = os.Getenv(prefix + "REPOS_PATH")
	} else {
		j.ReposPath = GitDefaultReposPath
	}
	if os.Getenv(prefix+"CACHE_PATH") != "" {
		j.CachePath = os.Getenv(prefix + "REPOS_PATH")
	} else {
		j.CachePath = GitDefaultCachePath
	}
	j.NoSSLVerify = StringToBool(os.Getenv(prefix + "NO_SSL_VERIFY"))
	if j.NoSSLVerify {
		NoSSLVerify()
	}
	return
}

// Validate - is current DS configuration OK?
func (j *DSGit) Validate() (err error) {
	url := strings.TrimSpace(j.URL)
	if strings.HasSuffix(url, "/") {
		url = url[:len(url)-1]
	}
	ary := strings.Split(url, "/")
	j.RepoName = ary[len(ary)-1]
	if j.RepoName == "" {
		err = fmt.Errorf("Repo name must be set")
	}
	j.ReposPath = os.ExpandEnv(j.ReposPath)
	if strings.HasSuffix(j.ReposPath, "/") {
		j.ReposPath = j.ReposPath[:len(j.ReposPath)-1]
	}
	j.CachePath = os.ExpandEnv(j.CachePath)
	if strings.HasSuffix(j.CachePath, "/") {
		j.CachePath = j.CachePath[:len(j.CachePath)-1]
	}
	return
}

// Name - return data source name
func (j *DSGit) Name() string {
	return j.DS
}

// Info - return DS configuration in a human readable form
func (j DSGit) Info() string {
	return fmt.Sprintf("%+v", j)
}

// CustomFetchRaw - is this datasource using custom fetch raw implementation?
func (j *DSGit) CustomFetchRaw() bool {
	return false
}

// FetchRaw - implement fetch raw data for git datasource
func (j *DSGit) FetchRaw(ctx *Ctx) (err error) {
	Printf("%s should use generic FetchRaw()\n", j.DS)
	return
}

// CustomEnrich - is this datasource using custom enrich implementation?
func (j *DSGit) CustomEnrich() bool {
	return false
}

// Enrich - implement enrich data for git datasource
func (j *DSGit) Enrich(ctx *Ctx) (err error) {
	Printf("%s should use generic Enrich()\n", j.DS)
	return
}

// GetGitOps - LOC, lang summary stats
func (j *DSGit) GetGitOps(ctx *Ctx, thrN int) (ch chan error, err error) {
	worker := func(c chan error, url string) (e error) {
		defer func() {
			if c != nil {
				c <- e
			}
		}()
		var (
			sout string
			serr string
		)
		cmdLine := []string{GitOpsCommand, url}
		var env map[string]string
		if GitOpsNoCleanup {
			env = map[string]string{"SKIP_CLEANUP": "1"}
		}
		sout, serr, e = ExecCommand(ctx, cmdLine, env)
		if e != nil {
			Printf("error executing %v: %v\n%s\n%s\n", cmdLine, e, sout, serr)
			return
		}
		type resultType struct {
			Loc int      `json:"loc"`
			Pls []RawPLS `json:"pls"`
		}
		var data resultType
		e = jsoniter.Unmarshal([]byte(sout), &data)
		if e != nil {
			Printf("error unmarshaling from %v\n", sout)
			return
		}
		j.Loc = data.Loc
		for _, f := range data.Pls {
			files, _ := strconv.Atoi(f.Files)
			blank, _ := strconv.Atoi(f.Blank)
			comment, _ := strconv.Atoi(f.Comment)
			code, _ := strconv.Atoi(f.Code)
			j.Pls = append(
				j.Pls,
				PLS{
					Language: f.Language,
					Files:    files,
					Blank:    blank,
					Comment:  comment,
					Code:     code,
				},
			)
		}
		return
	}
	if thrN <= 1 {
		return nil, worker(nil, j.URL)
	}
	ch = make(chan error)
	go func() { _ = worker(ch, j.URL) }()
	return ch, nil
}

// FetchItems - implement enrich data for git datasource
func (j *DSGit) FetchItems(ctx *Ctx) (err error) {
	// IMPL:
	var messages [][]byte
	// Process messages (possibly in threads)
	var (
		ch         chan error
		allMsgs    []interface{}
		allMsgsMtx *sync.Mutex
		escha      []chan error
		eschaMtx   *sync.Mutex
		goch       chan error
	)
	thrN := GetThreadsNum(ctx)
	if thrN > 1 {
		ch = make(chan error)
		allMsgsMtx = &sync.Mutex{}
		eschaMtx = &sync.Mutex{}
		goch, _ = j.GetGitOps(ctx, thrN)
	} else {
		_, err = j.GetGitOps(ctx, thrN)
		if err != nil {
			return
		}
	}
	// Do normal git processing, which don't needs gitops yet
	j.GitPath = j.ReposPath + "/" + j.URL + "-git"
	j.GitPath, err = EnsurePath(j.GitPath, true)
	FatalOnError(err)
	Printf("path to store git repository: %s\n", j.GitPath)
	// If MT allowed, wait for GitOps
	Printf("waiting for git ops result\n")
	if thrN > 1 {
		err = <-goch
		if err != nil {
			return
		}
	}
	Printf("loc: %d, programming languages summary: %+v\n", j.Loc, j.Pls)
	// Continue with operations that need git ops
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
		allMsgs = append(allMsgs, item)
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
func (j *DSGit) SupportDateFrom() bool {
	return true
}

// SupportOffsetFrom - does DS support resuming from offset?
func (j *DSGit) SupportOffsetFrom() bool {
	return false
}

// DateField - return date field used to detect where to restart from
func (j *DSGit) DateField(*Ctx) string {
	return DefaultDateField
}

// RichIDField - return rich ID field name
func (j *DSGit) RichIDField(*Ctx) string {
	return DefaultIDField
}

// RichAuthorField - return rich ID field name
func (j *DSGit) RichAuthorField(*Ctx) string {
	return DefaultAuthorField
}

// OffsetField - return offset field used to detect where to restart from
func (j *DSGit) OffsetField(*Ctx) string {
	return DefaultOffsetField
}

// OriginField - return origin field used to detect where to restart from
func (j *DSGit) OriginField(ctx *Ctx) string {
	if ctx.Tag != "" {
		return DefaultTagField
	}
	return DefaultOriginField
}

// Categories - return a set of configured categories
func (j *DSGit) Categories() map[string]struct{} {
	// IMPL:
	return map[string]struct{}{}
}

// ResumeNeedsOrigin - is origin field needed when resuming
// Origin should be needed when multiple configurations save to the same index
func (j *DSGit) ResumeNeedsOrigin(ctx *Ctx) bool {
	return !j.SingleOrigin
}

// Origin - return current origin
func (j *DSGit) Origin(ctx *Ctx) string {
	return j.URL
}

// ItemID - return unique identifier for an item
func (j *DSGit) ItemID(item interface{}) string {
	id, ok := item.(map[string]interface{})[Commit].(string)
	if !ok {
		Fatalf("%s: ItemID() - cannot extract %s from %+v", j.DS, Commit, DumpKeys(item))
	}
	return id
}

// AddMetadata - add metadata to the item
func (j *DSGit) AddMetadata(ctx *Ctx, item interface{}) (mItem map[string]interface{}) {
	// IMPL:
	mItem = make(map[string]interface{})
	origin := j.URL
	tag := ctx.Tag
	if tag == "" {
		tag = origin
	}
	itemID := j.ItemID(item)
	updatedOn := j.ItemUpdatedOn(item)
	uuid := UUIDNonEmpty(ctx, origin, itemID)
	timestamp := time.Now()
	mItem["backend_name"] = j.DS
	mItem["backend_version"] = GitBackendVersion
	mItem["timestamp"] = fmt.Sprintf("%.06f", float64(timestamp.UnixNano())/1.0e3)
	mItem[UUID] = uuid
	mItem[DefaultOriginField] = origin
	mItem[DefaultTagField] = tag
	mItem["updated_on"] = updatedOn
	mItem["category"] = j.ItemCategory(item)
	//mItem["search_fields"] = j.GenSearchFields(ctx, issue, uuid)
	//mItem["search_fields"] = make(map[string]interface{})
	mItem[DefaultDateField] = ToESDate(updatedOn)
	mItem[DefaultTimestampField] = ToESDate(timestamp)
	return
}

// ItemUpdatedOn - return updated on date for an item
func (j *DSGit) ItemUpdatedOn(item interface{}) time.Time {
	// IMPL:
	// should be CommitDate
	return time.Now()
}

// ItemCategory - return unique identifier for an item
func (j *DSGit) ItemCategory(item interface{}) string {
	return Commit
}

// ElasticRawMapping - Raw index mapping definition
func (j *DSGit) ElasticRawMapping() []byte {
	return GitRawMapping
}

// ElasticRichMapping - Rich index mapping definition
func (j *DSGit) ElasticRichMapping() []byte {
	return GitRichMapping
}

// GetItemIdentities return list of item's identities, each one is [3]string
// (name, username, email) tripples, special value Nil "<nil>" means null
// we use string and not *string which allows nil to allow usage as a map key
func (j *DSGit) GetItemIdentities(ctx *Ctx, doc interface{}) (map[[3]string]struct{}, error) {
	// IMPL:
	return map[[3]string]struct{}{}, nil
}

// GitEnrichItemsFunc - iterate items and enrich them
// items is a current pack of input items
// docs is a pointer to where extracted identities will be stored
func GitEnrichItemsFunc(ctx *Ctx, ds DS, thrN int, items []interface{}, docs *[]interface{}) (err error) {
	// IMPL:
	if ctx.Debug > 0 {
		Printf("stub enrich items %d/%d func\n", len(items), len(*docs))
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
func (j *DSGit) EnrichItems(ctx *Ctx) (err error) {
	Printf("enriching items\n")
	err = ForEachESItem(ctx, j, true, ESBulkUploadFunc, GitEnrichItemsFunc, nil)
	return
}

// EnrichItem - return rich item from raw item for a given author type
func (j *DSGit) EnrichItem(ctx *Ctx, item map[string]interface{}, author string, affs bool, extra interface{}) (rich map[string]interface{}, err error) {
	// IMPL:
	rich = item
	return
}

// AffsItems - return affiliations data items for given roles and date
func (j *DSGit) AffsItems(ctx *Ctx, rawItem map[string]interface{}, roles []string, date interface{}) (affsItems map[string]interface{}, err error) {
	// IMPL:
	return
}

// GetRoleIdentity - return identity data for a given role
func (j *DSGit) GetRoleIdentity(ctx *Ctx, item map[string]interface{}, role string) map[string]interface{} {
	// IMPL:
	return map[string]interface{}{"name": nil, "username": nil, "email": nil}
}

// AllRoles - return all roles defined for the backend
// roles can be static (always the same) or dynamic (per item)
// second return parameter is static mode (true/false)
// dynamic roles will use item to get its roles
func (j *DSGit) AllRoles(ctx *Ctx, item map[string]interface{}) ([]string, bool) {
	// IMPL:
	return []string{Author}, true
}
