import subprocess,time,json,os,sys,glob,collections,re
DUR=int(sys.argv[1]); SVC=int(sys.argv[2]); OUT=sys.argv[3]
WS=os.path.expanduser('~/.stapler-squad/workspaces/6eb0b580fa0331d5')
def ps():
    o=subprocess.run(['ps','-axo','pid=,ppid=,pcpu=,command='],capture_output=True,text=True).stdout
    r={}
    for l in o.splitlines():
        p=l.split(None,3)
        if len(p)==4: r[int(p[0])]=(int(p[1]),float(p[2]),p[3])
    return r
def desc(tab,root):
    kids=collections.defaultdict(list)
    for pid,(pp,_,_) in tab.items(): kids[pp].append(pid)
    out=set();st=[root]
    while st:
        x=st.pop()
        for k in kids[x]:
            if k not in out: out.add(k);st.append(k)
    return out
def files():
    r={}
    pats=[WS+'/*',WS+'/db/*',WS+'/logs/staplersquad.log',WS+'/checkpoints/*',os.path.expanduser('~/.stapler-squad/history_*.json'),os.path.expanduser('~/.stapler-squad/*.db*')]
    for p in pats:
        for f in glob.glob(p):
            try:
                s=os.stat(f); r[f]=(s.st_size,s.st_mtime)
            except OSError: pass
    return r
def mx(path):
    try: return subprocess.run(['curl','-s','-m5','localhost:8543'+path],capture_output=True,text=True).stdout
    except Exception: return ''
seen=set(); newrecs=[]; first=True
f0=files(); fl=f0; mt_changes=collections.Counter(); falcon=[]
open(OUT+'.m0','w').write(mx('/actuator/metrics')); open(OUT+'.g0','w').write(mx('/actuator/git-backend'))
t0=time.time(); nextf=t0
while time.time()-t0<DUR:
    tab=ps(); d=desc(tab,SVC)
    ids=set(tab)
    if first: seen=set(ids); first=False
    else:
        for pid in ids-seen:
            pp,c,cmd=tab[pid]
            newrecs.append((round(time.time()-t0,1),pid,pp,pp in d or pp==SVC,cmd[:200]))
        seen|=ids
    falcon.append(tab.get(575,(0,0,''))[1])
    if time.time()>=nextf:
        nf=files()
        for k,(sz,mt) in nf.items():
            if k in fl and fl[k][1]!=mt: mt_changes[k]+=1
        fl=nf; nextf+=5
    time.sleep(1.0)
t1=time.time()
open(OUT+'.m1','w').write(mx('/actuator/metrics')); open(OUT+'.g1','w').write(mx('/actuator/git-backend'))
f1=files()
json.dump({'dur':t1-t0,'new':newrecs,'falcon':falcon,'mt':mt_changes,
 'f0':{k:v for k,v in f0.items()},'f1':f1,'svc_desc_now':len(d)},open(OUT+'.json','w'))
