#!/usr/bin/env python3
"""Offline AST/source/provenance checker for the initial acceptance inventory.

Development-only reference audit; never called by the native Go SDK/CLI/MCP.
Reads the explicit public/synthetic source manifest only. No network, auth,
private-bank directory access, fixture generation, or Go implementation.
"""
import ast,json,pathlib,hashlib,re,collections,subprocess
root=pathlib.Path('/home/hermes/workspace/rental-monitoring/sber-sdk/fork')
go=pathlib.Path('/home/hermes/workspace/rental-monitoring/sber-go')
mat=json.loads((go/'docs/parity.json').read_text());man=json.loads((go/'docs/source-manifest.json').read_text())
files={x['relative_to_python_fork']:x for x in man['files']}
assert len(files)==len(man['files'])
lexical=[]; scoped_public=[]; raw_public=[]; klasses=[];testdefs=[]
def visit(n,path,parent=None,kind='module'):
 for ch in ast.iter_child_nodes(n):
  if isinstance(ch,(ast.FunctionDef,ast.AsyncFunctionDef,ast.ClassDef)):
   q=(parent+'.' if parent else '')+ch.name
   k=(path,ch.lineno,ch.name)
   if isinstance(ch,ast.ClassDef): klasses.append(k)
   else:
    lexical.append((k,q,kind=='function'))
    if not ch.name.startswith('_') and kind!='function': scoped_public.append(k)
   visit(ch,path,q,'class' if isinstance(ch,ast.ClassDef) else 'function')
  else:visit(ch,path,parent,kind)
for ref,f in files.items():
 data=pathlib.Path(f['path']).read_bytes()
 assert hashlib.sha256(data).hexdigest()==f['sha256'],ref
 if not ref.endswith('.py'):continue
 tree=ast.parse(data.decode(),filename=f['path'])
 if '/tests/' in '/'+ref:
  testdefs += [(ref,n.lineno,n.name) for n in ast.walk(tree) if isinstance(n,(ast.FunctionDef,ast.AsyncFunctionDef)) and n.name.startswith('test_')]
 else:
  visit(tree,ref)
  raw_public += [(ref,n.lineno,n.name) for n in ast.walk(tree) if isinstance(n,(ast.FunctionDef,ast.AsyncFunctionDef)) and not n.name.startswith('_')]
keys=lambda rows:{(r['source']['file'],r['source']['line'],r['qualified_name'].rsplit('.',1)[-1]) for r in rows}
inv=mat['inventory']['callables'];classes=mat['inventory']['classes']
assert len(inv)==len(keys(inv))==len(lexical)
assert keys(inv)=={x[0] for x in lexical}
assert keys([x for x in inv if x['public_named']])==set(scoped_public)
assert keys([x for x in inv if x['raw_ast_name_public']])==set(raw_public)
assert keys(classes)==set(klasses)
assert len(classes)==len(klasses)
criteria=mat['criteria'];ids=[x['id'] for x in criteria];assert len(ids)==len(set(ids))
assert all(x['status']=='pending' for x in criteria)
assert all(all(k in r for k in ('source','category','input_contract','output_contract','error_contract','permission_contract','required_go_tests')) for r in criteria)
assert all(r['required_go_tests'] for r in criteria)
required=[t for r in criteria for t in r['required_go_tests']]
assert all(re.fullmatch(r'(?:Test|Fuzz)[A-Za-z0-9_]+',t) for t in required)
assert len(required)==len(set(required)),[x for x,c in collections.Counter(required).items() if c>1]
src=ast.parse((root/'mcp/sber_unofficial_mcp/server.py').read_text())
actualtools=[n.name for n in src.body if isinstance(n,ast.AsyncFunctionDef) and n.name.startswith('sber_')]
tool_list_node=next(n for n in src.body if isinstance(n,ast.AnnAssign) and isinstance(n.target,ast.Name) and n.target.id=='_TOOLS')
assert isinstance(tool_list_node.value,ast.List)
listtools=[x.id for x in tool_list_node.value.elts if isinstance(x,ast.Name)]
assert len(listtools)==len(tool_list_node.value.elts)
assert len(actualtools)==len(set(actualtools))==len(listtools)==len(set(listtools))
assert set(actualtools)==set(listtools)==set(mat['inventory']['mcp']['defined_tools'])
for name in actualtools:
 item=next(x for x in inv if x['source']['file']=='mcp/sber_unofficial_mcp/server.py' and x['qualified_name']==name)
 row=next(x for x in criteria if x['id']==item['id'])
 banned={'login','password','pin','otp','captcha_code','deviceprint','antifraud_deviceprint','cookies','ufs_session','ufs_token'}
 assert not banned & {x['name'] for x in row['input_contract']['parameters']}
assert len(mat['inventory']['mcp']['default_registered_tools'])==9
assert len(mat['inventory']['mcp']['demo_registered_tools'])==6
assert len(mat['inventory']['mcp']['write_tools'])==5
catalog=json.loads((go/'testdata/compat/python-test-contracts.json').read_text())
tests=catalog['definitions'];tk={(x['source']['file'],x['source']['line'],x['id'].rsplit(':',1)[-1]) for x in tests}
assert tk==set(testdefs) and len(tests)==len(testdefs)
assert {r['id'] for r in criteria if r['id'].startswith('test:')}=={x['id'] for x in tests}
expanded=0
for t in tests:
 count=1
 for axis in t['input_contract']['parameter_axes']:
  assert axis['case_count'] is not None
  count*=axis['case_count']
 assert count==t['input_contract']['expanded_parameter_case_count'];expanded+=count
fixtureids=[];fixturecounts={}
for f in mat['fixture_files']:
 data=(go/f['path']).read_bytes();assert hashlib.sha256(data).hexdigest()==f['sha256']
 payload=json.loads(data);assert payload['synthetic_only'];rows=payload['records']
 assert f['record_ids']==[r['id'] for r in rows]
 fixturecounts[payload['family']]=len(rows)
 for r in rows:
  fixtureids.append((payload['family'],r['id']))
  assert r['status']=='pending' and r['provenance']
  for prov in r['provenance']:
   assert prov['file'].startswith('tests/'),prov
   assert files[prov['file']]['sha256']==prov['sha256']
   assert 1<=prov['line']<=prov['end_line']<=files[prov['file']]['line_count']
assert len(fixtureids)==len(set(fixtureids))
for r in criteria:
 s=r['source'];ref=files[s['file']]
 assert 1<=s['line']<=s['end_line']<=ref['line_count'],(r['id'],s)
head=subprocess.check_output(['git','rev-parse','HEAD'],cwd=root,text=True).strip()
staged=subprocess.check_output(['git','diff','--cached','--name-status'],cwd=root,text=True).splitlines()
assert head==man['canonical_head'];assert len(staged)==22
summary={'verified':True,'criteria_total':len(criteria),'all_pending':True,'unique_required_go_test_ids':len(required),'source_manifest_files':len(files),'sdk_modules':sum(x['relative_to_python_fork'].startswith('sber_unofficial/') and x['relative_to_python_fork'].endswith('.py') for x in man['files']),'all_source_lexical_callables':len(lexical),'ast_scoped_public_callables':len(scoped_public),'raw_ast_public_name_callables':len(raw_public),'nested_named_callables_not_external_public':len(raw_public)-len(scoped_public),'source_classes':len(klasses),'class_fields':sum(len(c['fields']) for c in classes),'explicit_exports':len(mat['inventory']['explicit_exports']),'inherited_public_methods':len(mat['inventory']['inherited_public_methods']),'constants':len(mat['inventory']['constants']),'mcp_tools':len(actualtools),'mcp_write_tools':len(mat['inventory']['mcp']['write_tools']),'mcp_default_tools':9,'mcp_demo_tools':6,'test_definitions':len(testdefs),'runtime_test_definitions':sum(t['category']=='synthetic_runtime_test' for t in tests),'publication_maintenance_test_definitions':sum(t['category']=='publication_maintenance_test' for t in tests),'expanded_parameter_cases':expanded,'fixture_records':len(fixtureids),'fixture_family_counts':fixturecounts,'endpoint_contracts':sum('http_method' in x for x in criteria),'cli_surface_rows':sum(x['visibility']=='cli_command' for x in criteria),'cross_layer_seams':sum(x['category']=='cross_layer_acceptance' for x in criteria),'missing_public_callables':[],'missing_mcp_tools':[],'missing_tests':[],'source_hashes_unchanged':True,'staged_repair_files':staged,'criteria_by_category':dict(sorted(collections.Counter(x['category'] for x in criteria).items())),'criteria_by_effect':dict(sorted(collections.Counter(x['effect'] for x in criteria).items())),'bank_or_network_requests_executed':False,'go_implementation_verified':False}
print(json.dumps(summary,ensure_ascii=False,indent=2))
