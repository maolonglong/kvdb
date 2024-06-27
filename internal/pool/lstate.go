package pool

import (
	"sync"

	luajson "github.com/alicebob/gopher-json"
	lua "github.com/yuin/gopher-lua"
)

var defaultLStatePool = newLStatePool(50, func() *lua.LState {
	lstate := lua.NewState(lua.Options{
		CallStackSize:       10,
		RegistrySize:        128,
		IncludeGoStackTrace: true,
		SkipOpenLibs:        true,
	})
	// Taken from the go-lua manual
	for _, pair := range []struct {
		f lua.LGFunction
		n string
	}{
		{lua.OpenPackage, lua.LoadLibName},
		{lua.OpenBase, lua.BaseLibName},
		{lua.OpenCoroutine, lua.CoroutineLibName},
		{lua.OpenTable, lua.TabLibName},
		{lua.OpenString, lua.StringLibName},
		{lua.OpenMath, lua.MathLibName},
		{lua.OpenDebug, lua.DebugLibName},
	} {
		lstate.Push(lstate.NewFunction(pair.f))
		lstate.Push(lua.LString(pair.n))
		lstate.Call(1, 0)
	}
	luajson.Preload(lstate)
	requireGlobal(lstate, "cjson", "json")
	_ = lstate.DoString(protectGlobals)
	return lstate
})

func GetLState() *lua.LState {
	return defaultLStatePool.Get()
}

func PutLState(lstate *lua.LState) {
	defaultLStatePool.Put(lstate)
}

type lstatePool struct {
	factory func() *lua.LState
	limit   chan struct{}
	pool    []*lua.LState
	m       sync.Mutex
}

func newLStatePool(size int, factory func() *lua.LState) *lstatePool {
	return &lstatePool{
		m:       sync.Mutex{},
		factory: factory,
		pool:    make([]*lua.LState, 0, size),
		limit:   make(chan struct{}, size),
	}
}

func (pl *lstatePool) Get() *lua.LState {
	pl.limit <- struct{}{}
	pl.m.Lock()
	defer pl.m.Unlock()
	n := len(pl.pool)
	if n == 0 {
		return pl.factory()
	}
	x := pl.pool[n-1]
	pl.pool = pl.pool[0 : n-1]
	return x
}

func (pl *lstatePool) Put(lstate *lua.LState) {
	pl.m.Lock()
	defer pl.m.Unlock()
	pl.pool = append(pl.pool, lstate)
	<-pl.limit
}

func (pl *lstatePool) Shutdown() {
	for _, L := range pl.pool {
		L.Close()
	}
}

// the following script protects globals
// it is based on:  http://metalua.luaforge.net/src/lib/strict.lua.html
var protectGlobals = `
local dbg=debug
local mt = {}
setmetatable(_G, mt)
mt.__newindex = function (t, n, v)
  if dbg.getinfo(2) then
    local w = dbg.getinfo(2, "S").what
    if w ~= "C" then
      error("Script attempted to create global variable '"..tostring(n).."'", 2)
    end
  end
  rawset(t, n, v)
end
mt.__index = function (t, n)
  if dbg.getinfo(2) and dbg.getinfo(2, "S").what ~= "C" then
    error("Script attempted to access nonexistent global variable '"..tostring(n).."'", 2)
  end
  return rawget(t, n)
end
debug = nil

`

func requireGlobal(l *lua.LState, id, modName string) {
	if err := l.CallByParam(lua.P{
		Fn:      l.GetGlobal("require"),
		NRet:    1,
		Protect: true,
	}, lua.LString(modName)); err != nil {
		panic(err)
	}
	mod := l.Get(-1)
	l.Pop(1)

	l.SetGlobal(id, mod)
}
