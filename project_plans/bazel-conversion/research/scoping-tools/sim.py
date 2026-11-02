from ta import *
import statistics, copy
B='tools/bzl/baseline/pr-sample.json'
PK=load()
FILES=fa.FILES
filemap={}  # (dir,name)->file rec
for f in FILES: filemap[(f['dir'],f['name'])]=f
def inmod(i): return i.startswith(MOD+'/') or i==MOD
class G:  # package graph with lib and test edges
    def __init__(s):
        s.lib={};s.test={};s.hastest=set();s.loc={};s.tloc={}
def base_graph():
    g=G()
    for p in PK:
        d=rel(p['ImportPath'])
        if isgen(d): continue
        g.lib[d]={rel(i) for i in (p.get('Imports') or []) if inmod(i) and not isgen(rel(i))}
        t={rel(i) for i in (p.get('TestImports') or [])+(p.get('XTestImports') or []) if inmod(i) and not isgen(rel(i))}
        g.test[d]=t
        if p.get('TestGoFiles') or p.get('XTestGoFiles'): g.hastest.add(d)
    for d in g.lib:
        g.loc[d]=sum(f['loc'] for f in FILES if f['dir']==d and not f['test'])
    return g
def affected(g,changed_libs,changed_tests):
    """changed_libs: set of pkg nodes whose non-test code changed; changed_tests: pkgs with only test change"""
    rev=collections.defaultdict(set)
    for p,im in g.lib.items():
        for i in im:
            if i in g.lib: rev[i].add(p)
    seen=set(changed_libs);st=list(seen)
    while st:
        x=st.pop()
        for y in rev.get(x,()):
            if y not in seen: seen.add(y);st.append(y)
    libs=seen
    tests={p for p in g.hastest if p in libs or (g.test[p]&libs)} | {p for p in changed_tests if p in g.hastest}
    return libs,tests
def metrics(g,changed_libs,changed_tests):
    libs,tests=affected(g,changed_libs,changed_tests)
    NL=len(g.lib);NT=len(g.hastest)
    # merged model (RA1 original): single graph lib∪test, pkgs
    return dict(libs=len(libs),tests=len(tests),tgt=(len(libs)+len(tests))/(NL+NT),loc=sum(g.loc[p] for p in libs)/sum(g.loc.values()),NL=NL,NT=NT)
def merged_frac(g,changed):
    E={p:set(g.lib[p])|set(g.test[p]) for p in g.lib}
    rev=collections.defaultdict(set)
    for p,im in E.items():
        for i in im:
            if i in E: rev[i].add(p)
    seen=set(changed)&set(E);st=list(seen)
    while st:
        x=st.pop()
        for y in rev.get(x,()):
            if y not in seen: seen.add(y);st.append(y)
    return len(seen)/len(E)
def pr_changes(path,node_of):
    """node_of(path)-> (set libs, set tests) from a repo-relative changed path"""
    return node_of(path)
def default_node_of(path,g,f2n=None):
    if path.endswith('.go'):
        d=os.path.dirname(path) or '.'
        name=os.path.basename(path)
        isT=name.endswith('_test.go')
        node=d
        if f2n and (d,name) in f2n: node=f2n[(d,name)]
        elif f2n and d in f2n.get('__split__',set()): node=f2n['__core__'][d]
        return ({node},set()) if not isT else (set(),{node})
    if path.startswith('proto/'):  # generated pkgs excluded -> dependents of gen: any importer. map to importers of gen
        return ('GEN',)
    if path.startswith('web-app/'):
        return ({'server/web'},set())
    return (set(),set())
