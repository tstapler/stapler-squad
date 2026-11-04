from scen import *
import ta
def cut(d,setfiles):
    A=ta.analyze(d);S_=set(setfiles)
    a2b=set();b2a=set();ua=set();ub=set()
    for a,bs in A['fe'].items():
        for b,syms in bs.items():
            if a not in S_ and b in S_: 
                a2b|={(b,)+s for s in syms}
            if a in S_ and b not in S_:
                b2a|={(a,)+s for s in syms}
    return dict(core_uses_set=len({x[1] for x in a2b}),set_uses_core=len({x[1] for x in b2a}),
        unexp_core_uses_set=len({x[1] for x in a2b if not x[2]}),unexp_set_uses_core=len({x[1] for x in b2a if not x[2]}),
        files_in_core_calling_set=len({x[0] for x in a2b}),files_in_set_calling_core=len({x[0] for x in b2a}))
def session_files(pred):
    return [f['name'] for f in FILES if f['dir']=='session' and not f['test'] and pred(f['name'])]
def run_all():
    sess,sinfo=mk_sats('session',SESS_NAMES);svc,vinfo=mk_sats('server/services',SVC_NAMES)
    inv=test_inversion_overrides()
    # stretch sets
    A=ta.analyze('session')
    bl=set(session_files(lambda n: n.startswith('backlog') or '_backlog' in n))
    # include receiver group of BacklogLifecycleListener
    for t,fs in A['meth_files'].items():
        if t in('BacklogLifecycleListener',): bl|=set(fs)
    store=set(session_files(lambda n: (n.startswith('ent_repository') or n.startswith('storage')) and n not in bl))
    rev=set(session_files(lambda n: n.startswith('review_') or n in('review_gate.go','review_queue_poller.go')))-bl
    st={}
    for nm,fs in (('backlog',bl),('store',store),('review',rev)):
        st[nm]=fs
    scen={
     'S0 baseline':({},[]),
     'S1 test back-edges':(dict(inv),[]),
     'S2 session mech satellites':(dict(sess),[]),
     'S3 services mech satellites':(dict(svc),[]),
     'S4 S1+S2+S3':({**sess,**svc,**inv},[]),
     'P2a +session/backlog (seam)':({**sess,**svc,**inv,**{('session',f):'session/backlog' for f in bl}},[('session','session/backlog')]),
     'P2b +session/store (seam)':({**sess,**svc,**inv,**{('session',f):'session/backlog' for f in bl},**{('session',f):'session/store' for f in store}},[('session','session/backlog'),('session','session/store')]),
     'P2c +session/review (seam)':({**sess,**svc,**inv,**{('session',f):'session/backlog' for f in bl},**{('session',f):'session/store' for f in store},**{('session',f):'session/review' for f in rev}},[('session','session/backlog'),('session','session/store'),('session','session/review')]),
    }
    results={}
    for k,(f2n,brk) in scen.items():
        g,node,sg,res=run(k,f2n)
        for core,n in brk:
            g.lib[core].discard(n)   # seam: core no longer imports the extracted package (interface in core)
        # recompute eval after edge break
        res=eval_graph(g,node,k)
        results[k]=(g,res,len(sg))
    base=results['S0 baseline'][1]
    print(f"{'scenario':32s} nodes testnodes | main500 meanLOC% (d) meanTLOC% fewer_loc | main100 meanLOC% fewerLOC | prs30 meanLOC% | med_tgt500")
    summ={}
    for k,(g,res,sgn) in results.items():
        pc=per_commit(base,res,k)
        summ[k]=pc
        m5=pc['main500'];m1=pc['main100'];p3=pc['prs30']
        print(f"{k:32s} {len(g.lib):4d} {len(g.hastest):4d} | {m5['mean_loc1']*100:5.1f} ({(m5['mean_loc1']-m5['mean_loc0'])*100:+5.1f}) {m5['mean_tloc1']*100:5.1f} {m5['fewer_loc']*100:4.1f}% | {m1['mean_loc1']*100:5.1f} {m1['fewer_loc']*100:4.1f}% | {p3['mean_loc1']*100:5.1f} | {m5['med_tgt1']}  (strad {sgn})")
    cuts={nm:cut('session',fs) for nm,fs in st.items()}
    for nm,c in cuts.items(): print(nm,len(st[nm]),'files',sum(filemap[('session',f)]['loc'] for f in st[nm]),'LOC',c)
    json.dump(summ,open('scen_summ.json','w'),indent=1)
    return results,summ,cuts,sinfo,vinfo,st
if __name__=='__main__': run_all()
