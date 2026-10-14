// Native PI1.2 SessionManager success/failure records; synthetic, no model calls.
import {pathToFileURL} from 'node:url';
import {mkdtempSync,copyFileSync,writeFileSync,readFileSync,rmSync} from 'node:fs';
import {join} from 'node:path';import {tmpdir} from 'node:os';
const base=join(process.env.NODE_PATH,'@earendil-works/pi-coding-agent');
if(JSON.parse(readFileSync(join(base,'package.json'),'utf8')).version!=='1.2.0')throw Error('PI1.2 required');
const {SessionManager}=await import(pathToFileURL(join(base,'dist/core/session-manager.js')));
const root=mkdtempSync(join(tmpdir(),'kit-pi-tool-results-'));
const manager=SessionManager.create('/synthetic/pi-tool-results',root);
manager.appendMessage({role:'user',content:'PI_TOOL_RESULT_CANARY',timestamp:Date.now()});
manager.appendMessage({role:'assistant',content:[{type:'toolCall',id:'synthetic-success',name:'synthetic_tool',arguments:{outcome:'success'}},{type:'toolCall',id:'synthetic-failure',name:'synthetic_tool',arguments:{outcome:'failure'}}],api:'anthropic-messages',provider:'synthetic',model:'synthetic-model',usage:{input:1,output:1,cacheRead:0,cacheWrite:0,totalTokens:2,cost:{input:0,output:0,cacheRead:0,cacheWrite:0,total:0}},stopReason:'toolUse',timestamp:Date.now()});
for(const isError of [false,true]) manager.appendMessage({role:'toolResult',toolCallId:isError?'synthetic-failure':'synthetic-success',toolName:'synthetic_tool',content:[{type:'text',text:isError?'PI_TOOL_FAILED result body':'PI_TOOL_SUCCESS result body'},{type:'text',text:'Additional native result block'}],isError,timestamp:Date.now()});
const output=process.argv[2]??'internal/trace/testdata/pi/tool-results-v3.jsonl';
copyFileSync(manager.getSessionFile(),output);
writeFileSync(output+'.provenance.json',JSON.stringify({package:'@earendil-works/pi-coding-agent',version:'1.2.0',generator:'scripts/generate-pi-tool-result-fixture.mjs',sessionId:manager.getSessionId(),entries:manager.getEntries().length},null,2)+'\n');
rmSync(root,{recursive:true});
