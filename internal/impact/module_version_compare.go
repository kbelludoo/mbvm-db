package impact

import (
 "context"
 "fmt"
 "os"
 "path/filepath"
 "sort"
 "time"
)

// CompareRealModuleDirs compiles the ACTUAL new provider and old clients
// against it, using readonly source overlays on the new snapshot.
// No hypothetical signature is generated and no client call is adapted.
// This first release checks ONE fully qualified function/method identity.
func CompareRealModuleDirs(oldRoot,newRoot,symbol string)(result RealModuleDiff,err error){
 result=RealModuleDiff{
  Schema:"mbvm-real-module-diff-v1",Symbol:symbol,
  OldDirectConsumers:[]string{},ConfirmedBrokenConsumers:[]string{},
  CompatibleInTest:[]string{},InconclusiveConsumers:[]string{},
  Consumers:[]RealVersionWitness{},
  Warnings:[]string{
   "Only old direct static call sites to one changed function are checked.",
   "Old client files are overlaid onto the real new tree; unchanged filename sets are required.",
   "Compiler confirmation requires a matching diagnostic at a typed old client call site.",
   "An inconclusive build is never treated as compatible or incompatible.",
   "Build variants, interface dispatch, reflection, cgo and external modules are not covered.",
  },
 }
 if symbol==""{return result,fmt.Errorf("a fully qualified -symbol is required")}
 oldRoot,err=filepath.Abs(oldRoot);if err!=nil{return result,err}
 newRoot,err=filepath.Abs(newRoot);if err!=nil{return result,err}
 beforeOld,err:=digestGoSourceTree(oldRoot);if err!=nil{return result,err}
 beforeNew,err:=digestGoSourceTree(newRoot);if err!=nil{return result,err}
 result.OldSourceSHA256=beforeOld;result.NewSourceSHA256=beforeNew
 defer func(){
  afterOld,e1:=digestGoSourceTree(oldRoot)
  afterNew,e2:=digestGoSourceTree(newRoot)
  if e1!=nil||e2!=nil||afterOld!=beforeOld||afterNew!=beforeNew{
   result.SourceFilesUnmodified=false
   if err==nil{err=fmt.Errorf("Go source trees changed during comparison: old=%v new=%v",e1,e2)}
   return
  }
  result.SourceFilesUnmodified=true
 }()
 oldModule,err:=LoadModule(oldRoot)
 if err!=nil{return result,fmt.Errorf("old module typecheck: %w",err)}
 oldProvider,id,err:=oldModule.FindSymbol(symbol)
 if err!=nil{return result,err}
 newDirs,err:=snapshotPackageDirs(newRoot)
 if err!=nil{return result,fmt.Errorf("new module discovery: %w",err)}
 newDir,ok:=newDirs[oldProvider.PackagePath]
 if !ok{return result,fmt.Errorf("provider import path %q absent from new module",oldProvider.PackagePath)}
 newProvider,err:=loadWithPackagePath(newDir,oldProvider.PackagePath)
 if err!=nil{return result,fmt.Errorf("new provider typecheck: %w",err)}
 newID:=-1
 for i,s:=range newProvider.Symbols{
  if s.Qualified()==symbol{newID=i;break}
 }
 if newID<0{return result,fmt.Errorf("new provider has removed %q; removal checks not supported",symbol)}
 result.OldContract,err=functionContract(oldProvider.Symbols[id])
 if err!=nil{return result,err}
 result.NewContract,err=functionContract(newProvider.Symbols[newID])
 if err!=nil{return result,err}
 result.ContractChanged=result.OldContract!=result.NewContract
 if !result.ContractChanged{return result,nil}
 consumers:=oldModule.DirectConsumerPackages(symbol)
 result.OldDirectConsumers=append(result.OldDirectConsumers,consumers...)
 if len(consumers)==0{return result,nil}
 tmp,err:=os.MkdirTemp("","mbvm-real-diff-")
 if err!=nil{return result,err}
 defer os.RemoveAll(tmp)
 newBuild:=&ModuleGraph{Root:newRoot,PackageDirs:newDirs}
 // Ensure the REAL new provider's exact source already builds, without
 // changing its declaration or making its own internal calls synthetic.
 ctx,cancel:=context.WithTimeout(context.Background(),2*time.Minute)
 providerOK,providerDiagnostics,providerErr:=newBuild.buildConsumer(ctx,
  oldProvider.PackagePath,filepath.Join(tmp,"new-provider"),"")
 cancel()
 result.ProviderBaselinePassed=providerOK
 if providerErr!=nil||!providerOK{
  reason:="real new provider does not compile"
  if providerErr!=nil{reason+=": "+providerErr.Error()}
  if len(providerDiagnostics)>0{reason+=": "+providerDiagnostics[0]}
  for _,pkg:=range consumers{
   result.InconclusiveConsumers=append(result.InconclusiveConsumers,pkg)
   result.Consumers=append(result.Consumers,RealVersionWitness{
    Package:pkg,Status:"inconclusive",Reason:reason,ConfirmedCompileErrors:[]string{},
    Diagnostics:providerDiagnostics,
   })
  }
  return result,nil
 }
 for index,pkg:=range consumers{
  witness:=RealVersionWitness{Package:pkg,Status:"inconclusive",
   ConfirmedCompileErrors:[]string{},Diagnostics:[]string{}}
  oldDir,oldFound:=oldModule.PackageDirs[pkg]
  newDir,newFound:=newDirs[pkg]
  if !oldFound||!newFound{
   witness.Reason="consumer package missing in one of the revisions"
  }else{
   oldGraph:=(*Graph)(nil)
   for _,g:=range oldModule.Packages{if g.PackagePath==pkg{oldGraph=g;break}}
   if oldGraph==nil{
    witness.Reason="no typed old consumer package"
   }else{
    ctx,cancel:=context.WithTimeout(context.Background(),2*time.Minute)
    baseline,_,e:=oldModule.buildConsumer(ctx,pkg,filepath.Join(tmp,fmt.Sprintf("old-%d",index)),"")
    cancel()
    witness.OldBaselinePassed=baseline
    if e!=nil||!baseline{
     witness.Reason="old consumer baseline build failed"
     if e!=nil{witness.Reason+=": "+e.Error()}
    }else{
     ctx,cancel=context.WithTimeout(context.Background(),2*time.Minute)
     baseline,_,e=newBuild.buildConsumer(ctx,pkg,filepath.Join(tmp,fmt.Sprintf("new-%d",index)),"")
     cancel()
     witness.NewBaselinePassed=baseline
     if e!=nil||!baseline{
      witness.Reason="new consumer baseline build failed"
      if e!=nil{witness.Reason+=": "+e.Error()}
     }else{
      dir:=filepath.Join(tmp,fmt.Sprintf("overlay-%03d",index))
      if e=os.MkdirAll(dir,0700);e!=nil{return result,e}
      overlay,overlayErr:=overlayOldClient(oldGraph,newDir,dir)
      if overlayErr!=nil{
       witness.Reason="old client overlay unsupported: "+overlayErr.Error()
      }else{
       ctx,cancel=context.WithTimeout(context.Background(),2*time.Minute)
       passed,diags,buildErr:=newBuild.buildConsumer(ctx,pkg,
        filepath.Join(tmp,fmt.Sprintf("mixed-%d",index)),overlay)
       cancel()
       witness.OldClientWithNewProviderPassed=passed
       witness.Diagnostics=diags
       if buildErr!=nil{
        witness.Reason="mixed build could not complete: "+buildErr.Error()
       }else if passed{
        witness.Status="compatible_in_test"
       }else{
        witness.ConfirmedCompileErrors=oldClientTargetErrors(oldModule,pkg,symbol,newRoot,diags)
        if len(witness.ConfirmedCompileErrors)>0{
         witness.Status="broken"
        }else{
         witness.Reason="no compiler error matching an old typed target call site"
        }
       }
      }
     }
    }
   }
   _=oldDir // path already verified by old module's typed loader
  }
  result.Consumers=append(result.Consumers,witness)
  switch witness.Status{
  case "broken":result.ConfirmedBrokenConsumers=append(result.ConfirmedBrokenConsumers,pkg)
  case "compatible_in_test":result.CompatibleInTest=append(result.CompatibleInTest,pkg)
  default:result.InconclusiveConsumers=append(result.InconclusiveConsumers,pkg)
  }
 }
 sort.Strings(result.ConfirmedBrokenConsumers)
 sort.Strings(result.CompatibleInTest)
 sort.Strings(result.InconclusiveConsumers)
 return result,nil
}
