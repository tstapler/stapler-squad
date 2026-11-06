import sys,json,urllib.request,urllib.parse
for expr in sys.argv[1:]:
    u="http://localhost:8428/api/v1/query?"+urllib.parse.urlencode({"query":expr})
    d=json.load(urllib.request.urlopen(u,timeout=20))
    rs=d["data"]["result"]
    print("==",expr)
    if not rs: print("  EMPTY")
    for r in rs[:12]:
        m={k:v for k,v in r["metric"].items() if k not in("job","otel_scope_name","__name__")}
        print("  ",m,r["value"][1])
