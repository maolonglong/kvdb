package core

import (
	"errors"
	"html"
	"io"
	"net/http"
	"net/url"
	"time"

	lua "github.com/yuin/gopher-lua"

	"github.com/maolonglong/kvdb/internal/kv"
	"github.com/maolonglong/kvdb/pkg/bytesconv"
)

const (
	_kvdbExitReason  = "__kvdb_exit_reason"
	_kvdbExitPayload = "__kvdb_exit_payload"
)

func mkLua(w io.Writer, r *http.Request, b *Bucket, txn any, l *lua.LState) *lua.LTable {
	write := func(newline bool) lua.LGFunction {
		return func(l *lua.LState) int {
			msg := l.CheckString(1)
			_, _ = w.Write(bytesconv.StringToBytes(msg))
			if newline {
				_, _ = w.Write([]byte{'\n'})
			}
			return 0
		}
	}
	fns := map[string]lua.LGFunction{
		"say":   write(true),
		"print": write(false),
		"redirect": func(l *lua.LState) int {
			url := l.CheckString(1)
			top := l.GetTop()
			code := http.StatusFound
			if top > 1 {
				code = l.CheckInt(2)
			}
			tb := l.NewTable()
			tb.RawSetString("url", lua.LString(url))
			tb.RawSetString("code", lua.LNumber(code))
			safeExit(l, "redirect", tb)
			return 0
		},
		"exit": func(l *lua.LState) int {
			code := l.CheckInt(1)
			safeExit(l, "exit", lua.LNumber(code))
			return 0
		},
		"get": func(l *lua.LState) int {
			key := l.CheckString(1)
			uKey := b.udataKey(bytesconv.StringToBytes(key), _markKeyValue)
			val, err := b.store.Get(r.Context(), txn, uKey)
			if err != nil {
				if errors.Is(err, kv.ErrKeyNotFound) {
					l.Push(lua.LNil)
					return 1
				}
				l.Error(lua.LString(err.Error()), 1)
				return 0
			}
			l.Push(lua.LString(val))
			return 1
		},
		"delete": func(l *lua.LState) int {
			key := l.CheckString(1)
			uKey := b.udataKey(bytesconv.StringToBytes(key), _markKeyValue)
			if err := b.store.Delete(r.Context(), txn, uKey); err != nil {
				l.Error(lua.LString(err.Error()), 1)
			}
			return 0
		},
		"set": func(l *lua.LState) int {
			key := l.CheckString(1)
			val := l.CheckString(2)
			var ttl int
			if l.GetTop() >= 3 {
				ttl = l.CheckInt(3)
			}
			uKey := b.udataKey(bytesconv.StringToBytes(key), _markKeyValue)
			opts := &kv.SetOptions{
				TTL: b.opts.DefaultTTL,
			}
			if ttl > 0 {
				opts.TTL = time.Duration(ttl) * time.Second
			}
			if err := b.store.Set(r.Context(), txn, uKey, bytesconv.StringToBytes(val), opts); err != nil {
				l.Error(lua.LString(err.Error()), 1)
				return 0
			}
			return 0
		},
		"incr": func(l *lua.LState) int {
			key := l.CheckString(1)
			increment := l.CheckInt64(2)
			var ttl int
			if l.GetTop() >= 3 {
				ttl = l.CheckInt(3)
			}
			uKey := b.udataKey(bytesconv.StringToBytes(key), _markKeyValue)
			opts := &kv.SetOptions{
				TTL: b.opts.DefaultTTL,
			}
			if ttl > 0 {
				opts.TTL = time.Duration(ttl) * time.Second
			}
			num, err := b.store.Incr(r.Context(), txn, uKey, increment, opts)
			if err != nil {
				l.Error(lua.LString(err.Error()), 1)
				return 0
			}
			l.Push(lua.LNumber(num))
			return 1
		},
		"escape_html": func(l *lua.LState) int {
			content := l.CheckString(1)
			l.Push(lua.LString(html.EscapeString(content)))
			return 1
		},
	}
	reqVar := l.NewTable()
	urlValuesToLua(l, reqVar, r.URL.Query())
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		if r.PostForm != nil {
			urlValuesToLua(l, reqVar, r.PostForm)
		}
	}
	vals := map[string]lua.LValue{
		"var":    reqVar,
		"status": lua.LNumber(200),
		"header": l.NewTable(),
	}

	mod := l.NewTable()
	for name, f := range fns {
		mod.RawSetString(name, l.NewFunction(f))
	}
	for name, v := range vals {
		mod.RawSetString(name, v)
	}
	return mod
}

func urlValuesToLua(l *lua.LState, tb *lua.LTable, m url.Values) {
	for k, v := range m {
		if len(v) == 1 {
			tb.RawSetString(k, lua.LString(v[0]))
		} else if len(v) > 0 {
			arr := l.NewTable()
			for i, s := range v {
				arr.RawSetInt(i, lua.LString(s))
			}
			tb.RawSetString(k, arr)
		}
	}
}

func safeExit(l *lua.LState, reason string, payload lua.LValue) {
	tb := l.NewTable()
	tb.RawSetString(_kvdbExitReason, lua.LString(reason))
	if payload != nil {
		tb.RawSetString(_kvdbExitPayload, payload)
	} else {
		tb.RawSetString(_kvdbExitPayload, lua.LNil)
	}
	l.Error(tb, 0)
}
