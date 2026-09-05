#!/usr/bin/env python3
"""启动与正式服务完全隔离的假数据网页预览；Ctrl+C 仅清理本次子进程。"""
import argparse
import datetime
import http.server
import json
import os
from pathlib import Path
import secrets
import shutil
import socketserver
import subprocess
import tempfile
import threading
import time
from urllib.parse import unquote

parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('--port',type=int,default=19080)
args=parser.parse_args()
root=Path(__file__).resolve().parent.parent
check=Path.home()/'.local/share/mihomo-loongnix/checks/web'
if not (check/'static/index.html').is_file():raise SystemExit('请先按 Web 接入文档构建检查产物')
subprocess.run(['go','build','-o',str(check/'mihomo-web'),'./cmd/mihomo-web'],cwd=root,check=True)
base=Path.home()/'.local/state/mihomo-loongnix-test/web'
base.mkdir(parents=True,exist_ok=True)
tmp=Path(tempfile.mkdtemp(prefix='preview-',dir=base))
now=lambda:datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')
profiles=[{'id':'demo','name':'日常连接','source':'https://example.invalid','active':True,'updated_at':'2026-09-06 01:00'}]
names=['香港 · 中环 01','香港 · 九龙 02','日本 · 东京 01','日本 · 大阪 02','新加坡 · 滨海 01','美国 · 洛杉矶 01','德国 · 法兰克福 01','英国 · 伦敦 01','台湾 · 台北 01']
groups=[{'name':'Auto','type':'Selector','now':names[0],'nodes':[{'name':n,'type':'Shadowsocks','delay':[32,45,78,92,115,185,-3,240,55][i]} for i,n in enumerate(names)]}]
status={'core':{'service_active':True,'controller_healthy':True,'running':True,'state_query_ok':True,'service_state':'active','pid':4242},'tun':{'configured':False,'runtime_enabled':False,'interface_present':False,'enabled':False,'observation_ok':True},'proxy_port':17890,'active_profile':profiles[0],'current_group':'Auto','current_node':names[0]}
logging={'enabled':False,'total_bytes':0,'current_file_bytes':0}
class Handler(http.server.BaseHTTPRequestHandler):
 def log_message(self,*args):pass
 def handle_request(self):
  path=unquote(self.path.split('?')[0]);body=json.loads(self.rfile.read(int(self.headers.get('Content-Length') or '0')) or '{}');data={}
  if path=='/v1/logs/stream':
   self.send_response(200);self.send_header('Content-Type','text/event-stream');self.end_headers()
   try:
    while True:
     self.wfile.write((json.dumps({'type':'info','payload':'[演示] 连接 example.invalid:443，通过 Auto 代理组。'},ensure_ascii=False)+'\n').encode());self.wfile.flush();time.sleep(2)
   except (BrokenPipeError,ConnectionResetError):return
  elif path=='/v1/status':status['observed_at']=now();data=status
  elif path.startswith('/v1/core/'):
   on=path.endswith('/start');status['core'].update(running=on,service_active=on,controller_healthy=on,service_state='active' if on else 'inactive');data=status
  elif path=='/v1/tun':status['tun'].update(configured=body['enabled'],enabled=body['enabled'] and status['core']['running'],runtime_enabled=body['enabled'],interface_present=body['enabled']);data=status
  elif path=='/v1/proxy-port':status['proxy_port']=body['port'];data=status
  elif path=='/v1/profiles':
   if self.command=='POST':profiles.append({'id':secrets.token_hex(6),'name':body.get('name') or '新配置','source':'https://example.invalid','active':False});data=profiles[-1]
   else:data=profiles
  elif path.startswith('/v1/profiles/'):
   parts=path.split('/');p=next((p for p in profiles if p['id']==parts[3]),None)
   if p:
    if self.command=='DELETE':profiles.remove(p);data=profiles
    elif self.command=='PATCH':p['name']=body['name'];data=p
    elif path.endswith('/activate'):
     for q in profiles:q['active']=q is p
     status['active_profile']=p;data=p
    else:data=p
  elif path=='/v1/proxy-groups':data=groups
  elif path.startswith('/v1/proxy-groups/'):
   groups[0]['now']=body['name'];status['current_node']=body['name'];data=groups[0]
  elif path=='/v1/proxy-delay':time.sleep(.15);data={'name':body['name'],'delay':48}
  elif path=='/v1/rules':data=[{'type':t,'content':c,'policy':p} for t,c,p in [('DomainSuffix','example.invalid','Auto'),('IPCIDR','192.168.0.0/16','DIRECT'),('DomainSuffix','internal.invalid','DIRECT'),('MATCH','','Auto')]]
  elif path in ('/v1/logging','/v1/logging/status'):
   if self.command=='PUT':logging['enabled']=body['enabled']
   data=logging
  else:self.send_error(404);return
  self.send_response(200);self.send_header('Content-Type','application/json');self.end_headers();self.wfile.write(json.dumps({'success':True,'data':data},ensure_ascii=False).encode())
 do_GET=do_POST=do_PUT=do_PATCH=do_DELETE=handle_request
class UnixServer(socketserver.ThreadingMixIn,socketserver.UnixStreamServer):daemon_threads=True
up=UnixServer(str(tmp/'manager.sock'),Handler)
password='preview-only-mihomo'
hash_value=subprocess.run([str(check/'mihomo-web'),'--hash-password'],input=password,text=True,stdout=subprocess.PIPE,check=True).stdout.strip()
config={'listen':f'127.0.0.1:{args.port}','public_url':f'http://127.0.0.1:{args.port}','manager_socket':str(tmp/'manager.sock'),'password_hash':hash_value,'summary_token':secrets.token_hex(32),'show_node':True,'test_mode':True}
(tmp/'config.json').write_text(json.dumps(config));(tmp/'config.json').chmod(0o600)
threading.Thread(target=up.serve_forever,daemon=True).start()
child=None
try:
 child=subprocess.Popen([str(check/'mihomo-web'),'--config',str(tmp/'config.json'),'--static',str(check/'static')])
 print(f'独立假数据预览：http://127.0.0.1:{args.port}；演示密码：{password}',flush=True)
 child.wait()
except KeyboardInterrupt:pass
finally:
 if child and child.poll() is None:
  child.terminate()
  try:child.wait(timeout=8)
  except subprocess.TimeoutExpired:child.kill();child.wait()
 up.shutdown();up.server_close();shutil.rmtree(tmp)
