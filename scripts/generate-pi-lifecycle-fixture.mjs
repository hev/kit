// Actual PI1.2 AgentSession lifecycle, deterministic local stream, isolated state.
// NODE_PATH must point at a disposable PI1.2 npm installation.
import { pathToFileURL } from 'node:url';
import { mkdtempSync, mkdirSync, copyFileSync, writeFileSync, readFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
const base=join(process.env.NODE_PATH,'@earendil-works');
const version=JSON.parse(readFileSync(join(base,'pi-coding-agent/package.json'),'utf8')).version;
if(version!=='1.2.0') throw new Error('PI1.2.0 required');
const pi=await import(pathToFileURL(join(base,'pi-coding-agent/dist/index.js')));
const ai=await import(pathToFileURL(join(base,'pi-ai/dist/compat.js')));
const root=mkdtempSync(join(tmpdir(),'kit-pi-lifecycle-'));
const cwd=join(root,'cwd'),agentDir=join(root,'agent');mkdirSync(cwd);mkdirSync(agentDir);
const runtime=await pi.ModelRuntime.create({authPath:join(agentDir,'auth.json'),modelsPath:null,modelsStorePath:join(agentDir,'models-store.json'),refreshOnCreate:false,allowModelNetwork:false});
await runtime.setRuntimeApiKey('anthropic','synthetic-not-a-real-key');
const settings=pi.SettingsManager.inMemory();
const loader=new pi.DefaultResourceLoader({cwd,agentDir,settingsManager:settings,noExtensions:true,noSkills:true,noPromptTemplates:true,noThemes:true,noContextFiles:true,systemPrompt:'PI_LIFECYCLE_SYSTEM synthetic instructions'});
await loader.reload();
const manager=pi.SessionManager.create(cwd,join(root,'sessions'));
const model=ai.getModel('anthropic','claude-sonnet-4-5');
const {session}=await pi.createAgentSession({cwd,agentDir,model,modelRuntime:runtime,settingsManager:settings,resourceLoader:loader,sessionManager:manager,noTools:'all'});
session.agent.streamFunction=()=>{
 const stream=ai.createAssistantMessageEventStream();
 const message={role:'assistant',content:[{type:'text',text:'PI_LIFECYCLE_ANSWER synthetic'}],api:model.api,provider:model.provider,model:model.id,usage:{input:11,output:7,cacheRead:0,cacheWrite:0,totalTokens:18,cost:{input:0,output:0,cacheRead:0,cacheWrite:0,total:0}},stopReason:'stop',timestamp:Date.now()};
 queueMicrotask(()=>{stream.push({type:'done',reason:'stop',message});stream.end(message)});return stream;
};
await session.prompt('PI_LIFECYCLE_CANARY synthetic user');
manager.appendMessage({role:'system',content:[{type:'text',text:'PI_LIFECYCLE_UPDATE'}],sections:{'synthetic.instructions':'PI_LIFECYCLE_SECTION','synthetic.removed':null},toolsAdded:[{name:'synthetic_tool',description:'Synthetic tool',parameters:{type:'object',properties:{}}}],toolsRemoved:[{name:'old_synthetic_tool'}],timestamp:Date.now()});
// Flush the update through another actual prompt/assistant lifecycle.
await session.prompt('PI_LIFECYCLE_SECOND synthetic user');
session.dispose();
const output=process.argv[2]??'internal/trace/testdata/pi/lifecycle-v3.jsonl';
copyFileSync(manager.getSessionFile(),output);
writeFileSync(output+'.provenance.json',JSON.stringify({package:'@earendil-works/pi-coding-agent',version,generator:'scripts/generate-pi-lifecycle-fixture.mjs',method:'createAgentSession.prompt with deterministic injected stream; isolated resources/auth; no network model calls',sessionId:manager.getSessionId(),entries:manager.getEntries().length},null,2)+'\n');
rmSync(root,{recursive:true});
