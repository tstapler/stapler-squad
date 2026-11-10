import ctypes,subprocess,time,sys,os
lib=ctypes.CDLL('/usr/lib/libproc.dylib')
class RU(ctypes.Structure):
    _fields_=[('uuid',ctypes.c_ubyte*16)]+[(n,ctypes.c_uint64) for n in 'user sys pkg intr pageins wired res foot start exit cu cs cpkg cintr cpageins celapsed bread bwritten'.split()]
def rd(pid):
    r=RU()
    if lib.proc_pid_rusage(pid,2,ctypes.byref(r))!=0: return None
    return r
def snap():
    out=subprocess.run(['ps','-axo','pid=,user=,comm='],capture_output=True,text=True).stdout.splitlines()
    s={}
    for l in out:
        p=l.split(None,2)
        if len(p)<3: continue
        r=rd(int(p[0]))
        if r: s[int(p[0])]=(r.start,r.bread,r.bwritten,p[2])
    return s
D=int(sys.argv[1]); a=snap(); t=time.time(); time.sleep(D); b=snap(); el=time.time()-t
rows=[]
for pid,(st,br,bw,c) in b.items():
    if pid in a and a[pid][0]==st:
        rows.append(((bw-a[pid][2])/el,(br-a[pid][1])/el,pid,c))
rows.sort(reverse=True)
print(f'window {el:.0f}s; total written {sum(r[0] for r in rows)/1024:.0f} KB/s (user-visible pids), read {sum(r[1] for r in rows)/1024:.0f} KB/s')
for w,r,pid,c in rows[:14]: print(f'{w/1024:9.1f} KB/s wr {r/1024:9.1f} KB/s rd  pid {pid} {os.path.basename(c.split(" ")[0]) if "/" in c else c}'[:150], c[-60:] if 'systemextension' in c else '')
