import sqlite3,os,sys,time,ctypes,random,secrets,struct
lib=ctypes.CDLL('/usr/lib/libproc.dylib')
class RU(ctypes.Structure):
    _fields_=[('uuid',ctypes.c_ubyte*16)]+[(n,ctypes.c_uint64) for n in 'user sys pkg intr pageins wired res foot start exit cu cs cpkg cintr cpageins celapsed bread bwritten'.split()]
def bw():
    r=RU(); lib.proc_pid_rusage(os.getpid(),2,ctypes.byref(r)); return r.bwritten
SCHEMA="""CREATE TABLE escape_events (id text NOT NULL, session_id text NOT NULL, project_path text NULL, stage text NOT NULL, sequence_type text NOT NULL, sequence_subtype text NULL, sequence_signature text NULL, byte_length integer NOT NULL, payload_hash text NULL, raw_bytes blob NULL, mangled bool NOT NULL DEFAULT (false), mangle_type text NULL, wall_time datetime NOT NULL, session_seq integer NOT NULL, PRIMARY KEY (id));
CREATE INDEX a ON escape_events (session_id, session_seq); CREATE INDEX b ON escape_events (session_id, wall_time); CREATE INDEX c ON escape_events (wall_time);"""
EVERY=int(sys.argv[4]); mode=sys.argv[1]; S=118; CAP=int(sys.argv[2]); BATCHES=int(sys.argv[3]); path=f'/tmp/edr-exp/work/idbench-{mode}.db'
for s in('','-wal','-shm'):
    try: os.remove(path+s)
    except OSError: pass
db=sqlite3.connect(path,isolation_level=None); db.execute('pragma journal_mode=wal'); db.execute('pragma synchronous=normal'); db.executescript(SCHEMA)
ctr=[secrets.randbits(24)]
def nid():
    if mode=='random': return secrets.token_hex(8)
    ctr[0]=(ctr[0]+1)&0xFFFFFF
    return '%010x%06x'%(int(time.time()*1000)&0xFFFFFFFFFF,ctr[0])
seq=[0]*S; t=[0]
def row(si):
    seq[si]+=1; t[0]+=1
    return (nid(),f's{si}',None,'pty_read','SGR','m',f'sig{seq[si]%50}',seq[si]%40+3,None,None,0,None,'2026-11-09 12:00:%09d'%t[0],seq[si])
INS="insert into escape_events values(?,?,?,?,?,?,?,?,?,?,?,?,?,?)"
db.execute('begin')
for _ in range(CAP):
    db.executemany(INS,[row(si) for si in range(S)])
db.execute('commit'); db.execute('pragma wal_checkpoint(truncate)')
b0=bw(); t0=time.time(); rows=0
for _ in range(BATCHES):
    batch=[row(random.randrange(S)) for _ in range(500)]
    db.execute('begin'); db.executemany(INS,batch); db.execute('commit'); rows+=500
    if (_+1)%EVERY: continue
    for si in range(S):
        over=seq[si]-CAP-0  # rows over cap for this session
        cur=db.execute('select count(*) from escape_events where session_id=?',(f's{si}',)).fetchone()[0]
        over=cur-CAP
        while over>0:
            ids=[r[0] for r in db.execute('select id from escape_events where session_id=? order by wall_time limit ?',(f's{si}',min(over,250)))]
            db.execute('delete from escape_events where id in (%s)'%','.join('?'*len(ids)),ids); over-=len(ids)
db.execute('pragma wal_checkpoint(truncate)')
w=bw()-b0
print(f'{mode:8} cap/session={CAP} rows inserted={rows} bytes written={w/1e6:.1f} MB  => {w/rows:.0f} B per inserted row (incl. eviction+checkpoint), {time.time()-t0:.0f}s')
