package impact

import (
 "fmt"
 "path/filepath"
 "sync"
 "time"
)

// ModuleCache is an explicit per-process optimization for repeated,
// read-only impact queries. It never returns mutable graph internals.
// LoadModule and LoadModuleWithStats always perform fresh loads.
type ModuleCache struct{
 mu sync.Mutex
 entries map[string]cachedModule
 order []string
 capacity int
}
type cachedModule struct{
 fingerprint string
 graph *ModuleGraph
}
type ModuleCacheQueryStats struct{
 CacheHit bool `json:"cache_hit"`
 CacheBypassedReason string `json:"cache_bypassed_reason,omitempty"`
 FingerprintMilliseconds float64 `json:"fingerprint_ms"`
 TotalMilliseconds float64 `json:"total_ms"`
 GoListCalls int `json:"go_list_calls"`
 Packages int `json:"packages"`
 Symbols int `json:"symbols"`
 Edges int `json:"edges"`
}

// NewModuleCache constructs a bounded cache. A nonpositive capacity
// defaults to two module snapshots. Callers should keep this object in a
// long-running process to benefit from cache hits; single-shot CLI runs
// gain nothing. No graph or compiler objects are persisted on disk.
func NewModuleCache(capacity int)*ModuleCache{
 if capacity<=0{capacity=2}
 return &ModuleCache{
  entries:make(map[string]cachedModule),
  capacity:capacity,
 }
}

// Impact returns a fresh report, but caches the private typed graph.
// All access (including building) is serialized by a mutex. This avoids
// duplicate builds and protects the shared AST/importer from races.
func (c *ModuleCache) Impact(root,symbol string)(ModuleImpact,ModuleCacheQueryStats,error){
 result:=ModuleImpact{}
 stats:=ModuleCacheQueryStats{}
 if c==nil{return result,stats,fmt.Errorf("nil module cache")}
 start:=time.Now()
 c.mu.Lock()
 defer c.mu.Unlock()
 abs,err:=filepath.Abs(root)
 if err!=nil{return result,stats,err}
 fingerprintStart:=time.Now()
 fingerprint,err:=fingerprintModule(abs)
 stats.FingerprintMilliseconds=durationMillis(time.Since(fingerprintStart))
 if err!=nil{return result,stats,err}
 if !fingerprint.cacheable{
  // Unknown import behavior? Use full-resolution Go tooling as before.
  stats.CacheBypassedReason=fingerprint.reason
 }
 graph:=(*ModuleGraph)(nil)
 if fingerprint.cacheable{
  if entry,ok:=c.entries[abs];ok&&entry.fingerprint==fingerprint.hash{
   graph=entry.graph
   stats.CacheHit=true
   c.markRecent(abs)
  }
 }
 if graph==nil{
  var loadStats ModuleLoadStats
  graph,loadStats,err=LoadModuleWithStats(abs)
  stats.GoListCalls=loadStats.GoListCalls
  if err!=nil{return result,stats,err}
  // Only publish graphs built from stable source/configuration inputs.
  // No partial result is inserted when a source changes during load.
  validation,err:=fingerprintModule(abs)
  if err!=nil{return result,stats,err}
  if fingerprint.cacheable&&validation.cacheable&&fingerprint.hash!=validation.hash{
   return result,stats,fmt.Errorf("module inputs changed during cache load; retry")
  }
  if fingerprint.cacheable&&validation.cacheable{
   c.entries[abs]=cachedModule{fingerprint:validation.hash,graph:graph}
   c.markRecent(abs)
   for len(c.order)>c.capacity{
    oldest:=c.order[0]
    c.order=c.order[1:]
    delete(c.entries,oldest)
   }
  }
 }
 result,err=graph.Impact(symbol)
 stats.Packages=len(graph.Packages)
 stats.Symbols=len(graph.Symbols)
 stats.Edges=len(graph.Edges)
 stats.TotalMilliseconds=durationMillis(time.Since(start))
 return result,stats,err
}

// markRecent runs exclusively with the cache's mutex held.
func (c *ModuleCache) markRecent(root string){
 for i,key:=range c.order{
  if key==root{
   c.order=append(c.order[:i],c.order[i+1:]...)
   break
  }
 }
 c.order=append(c.order,root)
}

// Invalidate removes a cached module explicitly. The next read will
// rescan the source tree and invoke the Go compiler exporter again.
func (c *ModuleCache) Invalidate(root string)error{
 if c==nil{return fmt.Errorf("nil module cache")}
 abs,err:=filepath.Abs(root)
 if err!=nil{return err}
 c.mu.Lock()
 defer c.mu.Unlock()
 delete(c.entries,abs)
 for i,key:=range c.order{
  if key==abs{c.order=append(c.order[:i],c.order[i+1:]...);break}
 }
 return nil
}

// Clear frees cached graphs and compiler objects. It never removes
// source files, module artifacts or the Go build cache.
func (c *ModuleCache) Clear(){
 if c==nil{return}
 c.mu.Lock()
 defer c.mu.Unlock()
 c.entries=make(map[string]cachedModule)
 c.order=nil
}
