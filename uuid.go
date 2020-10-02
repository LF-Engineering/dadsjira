package dads

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"sync"

	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var (
	// uuidsCacheNonEmpty caches UUIDNonEmpty calls
	uuidsCacheNonEmpty    = map[string]string{}
	uuidsCacheNonEmptyMtx *sync.RWMutex
	// uuidsCacheAffs caches UUIDAffs calls
	uuidsCacheAffs    = map[string]string{}
	uuidsCacheAffsMtx *sync.RWMutex
)

// UUIDNonEmpty - generate UUID of string args (all must be non-empty)
// uses internal cache
func UUIDNonEmpty(ctx *Ctx, args ...string) (h string) {
	k := strings.Join(args, ":")
	uuidsCacheNonEmptyMtx.RLock()
	h, ok := uuidsCacheNonEmpty[k]
	uuidsCacheNonEmptyMtx.RUnlock()
	if ok {
		return
	}
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
	uuidsCacheNonEmptyMtx.Lock()
	uuidsCacheNonEmpty[k] = h
	uuidsCacheNonEmptyMtx.Unlock()
	return
}

// UUIDAffs - generate UUID of string args
// uses internal cache
// downcases arguments, all but first can be empty
// if argument is Nil "<nil>" replaces with "None"
func UUIDAffs(ctx *Ctx, args ...string) (h string) {
	k := strings.Join(args, ":")
	uuidsCacheAffsMtx.RLock()
	h, ok := uuidsCacheAffs[k]
	uuidsCacheAffsMtx.RUnlock()
	if ok {
		return
	}
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
	uuidsCacheAffsMtx.Lock()
	uuidsCacheAffs[k] = h
	uuidsCacheAffsMtx.Unlock()
	return
}
