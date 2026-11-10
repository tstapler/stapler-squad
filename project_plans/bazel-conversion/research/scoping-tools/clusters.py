from scen2 import *
def cluster_report(d,names,minfiles=3):
    f2n,info=mk_sats(d,names,minfiles)
    g,node,strad=build(f2n)
    stm={k:v for k,v in strad}
    A=ta.analyze(d)
    out=[]
    for nm,fl in info:
        S_=set(fl)
        exp_syms=set();ext_files=set();ext_pkgs=set()
        for e in EDGES:
            if rp(e['DP'])==d and os.path.basename(e['DF']) in S_ and os.path.dirname(e['DF'])==d:
                up=rp(e['UP'])
                if up!=d and e['Exp'] and e['Kind'] in('func','type','var','const','method'):
                    ext_files.add(e['UF']);ext_pkgs.add(up)
                if e['Exp'] and e['Kind'] in('func','type','var','const'): exp_syms.add(e['Sym'])
        # moved symbols exported declared in cluster
        decl_exp={x['n'] for f in A['nt'] if False for x in []}
        for f in FILES:
            if f['dir']==d and f['name'] in S_ and not f['test']:
                for x in f['decls'] or []:
                    if x['n'][:1].isupper() and x['k']!='method': decl_exp.add(x['n'])
        tests=[k[1] for k,n in node.items() if n==nm and k[1].endswith('_test.go')]
        tstr=[(k[1],v) for k,v in strad if node.get(k)==nm]
        # unexported crossing from cluster -> rest of pkg (needs export), and tests crossing
        outd,inn=boundary_fe(A,fl)
        out.append(dict(pkg=nm,files=len(fl),loc=sum(filemap[(d,f)]['loc'] for f in fl),tests=len(tests),test_loc=sum(filemap[(d,t)]['loc'] for t in tests),
            exported_decls=len(decl_exp),ext_importer_pkgs=sorted(ext_pkgs),ext_importer_files=len(ext_files),
            to_export=outd,core_uses_cluster=inn,tests_straddling=len(tstr),file_list=sorted(fl)))
    return out
def boundary_fe(A,fl):
    S_=set(fl);o=set();i=set()
    for a,bs in A['fe'].items():
        for b,syms in bs.items():
            if a in S_ and b not in S_: o|={s for s in syms if not s[1]}
            if b in S_ and a not in S_: i|={s for s in syms if not s[1]}
    return len(o),len(i)
if __name__=='__main__':
    res={}
    for d,names in (('session',SESS_NAMES),('server/services',SVC_NAMES)):
        r=cluster_report(d,names);res[d]=r
        for c in r:
            print(f"{c['pkg']:38s} files={c['files']:2d} loc={c['loc']:5d} tests={c['tests']:2d}({c['test_loc']}) exportedDecls={c['exported_decls']:3d} extImporterFiles={c['ext_importer_files']:3d} pkgs={c['ext_importer_pkgs']} unexp->export={c['to_export']} coreUsesUnexp={c['core_uses_cluster']} straddlingTests={c['tests_straddling']}")
    json.dump(res,open('clusters.json','w'),indent=1)
