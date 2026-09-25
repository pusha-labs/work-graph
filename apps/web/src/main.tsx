import { createContext, FormEvent, Fragment, ReactNode, StrictMode, useContext, useEffect, useMemo, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import './styles.css';

type Readiness = 'checking' | 'ready' | 'not-ready';
type Account = { id: string; email: string; displayName: string };
type AuthStatus = { setupRequired: boolean; registrationEnabled: boolean; passwordResetEnabled:boolean;authenticated: boolean; account: Account | null };
type Workspace = { id: string; name: string; revision: number; workDistributionMode:'simple'|'exchange'; createdAt: string };
type WorkNode = {
  id: string;
  workspaceId: string;
  rootId: string;
  parentId: string | null;
  title: string;
  desiredOutcome: string;
  lifecycleStatus: 'planned' | 'active' | 'blocked' | 'review' | 'closed';
  createdRevision: number;
  updatedRevision: number;
};
type ChangeEvent = {
  id: string;
  workspaceRevision: number;
  entityId: string;
  eventType: 'node.created' | 'node.updated';
  beforeState: WorkNode | null;
  afterState: WorkNode;
  occurredAt: string;
};
type Capability = { id: string; name: string; capabilityType: 'role' | 'skill'; createdAt: string; sources?:('self'|'admin'|'inferred'|'imported')[] };
type Actor = { id: string; displayName: string; actorType: 'person' | 'automation'; hasAccount: boolean; workspaceRole?: 'owner'|'admin'|'member'; isCurrent:boolean; createdAt: string; capabilities: Capability[] };
type Directory = { actors: Actor[]; capabilities: Capability[]; currentWorkspaceRole: 'owner' | 'admin' | 'member' };
type Invitation = { id: string; workspaceId: string; workspaceName?: string; email: string; displayName: string; workspaceRole: 'admin' | 'member'; expiresAt: string; acceptedAt: string | null; createdAt: string };
type ClaimEvidence={source:'self'|'admin'|'inferred'|'imported';level:'awareness'|'working'|'advanced'|'expert';note:string;reviewedAt:string};
type CapabilityClaim = { id: string; name: string; capabilityType: 'role' | 'skill'; sources: ClaimEvidence['source'][]; evidence:ClaimEvidence[] };
type KnowledgeClaim = { id:string; name:string; subjectType:KnowledgeSubject['subjectType']; sources:ClaimEvidence['source'][]; evidence:ClaimEvidence[] };
type EstimationStats={observationCount:number;meanRelativeError:number;meanAbsoluteRelativeError:number;stability:number;longOverrunCount:number;longOverrunRate:number;meanLongOverrunSeverity:number;updatedAt:string|null};
type MyProfile = { id: string; displayName: string; actorType: string; workspaceRole: 'owner' | 'admin' | 'member'; claims: CapabilityClaim[]; knowledge:KnowledgeClaim[]; estimation:EstimationStats; createdAt: string };
type KnowledgeHolder = { id: string; displayName: string; sources: ('self' | 'admin' | 'inferred' | 'imported')[] };
type KnowledgeSubject = { id: string; name: string; subjectType: 'service' | 'project' | 'system' | 'domain' | 'other'; createdAt: string; holders: KnowledgeHolder[] };
type ActorSummary = { id: string; displayName: string; actorType: string };
type DescendantBlocker = Pick<WorkNode, 'id' | 'title' | 'lifecycleStatus'> & { depth: number };
type StepType = 'human' | 'api' | 'script' | 'module';
type WorkflowExecution = { id:string; attemptNumber:number; executionStatus:'running'|'succeeded'|'returned'|'superseded'|'failed'|'cancelled'|'timed_out'; startedBy:ActorSummary|null; startedAt:string; finishedAt:string|null; result:Record<string,unknown>; errorMessage:string|null; returnReason:string|null; definitionSnapshot:Record<string,unknown>;promisedDurationMinutes:number|null;actualDurationMinutes:number|null;relativeError:number|null };
type DistributionMode='inherit'|'simple'|'exchange';
type WorkflowStep = { id: string; position: number; name: string; stepType: StepType; stepStatus: 'pending' | 'ready' | 'assigned' | 'active' | 'completed' | 'failed'; moduleId: string | null; moduleVersion: string | null; configuration: Record<string, unknown>; distributionMode:DistributionMode; effectiveDistributionMode:'simple'|'exchange'; claimedBy: ActorSummary | null; requirements: Capability[]; knowledgeRequirements: Pick<KnowledgeSubject, 'id' | 'name' | 'subjectType'>[]; executions:WorkflowExecution[];bids:StepBid[] };
type NewWorkflowStep = { name: string; stepType: StepType; capabilityId?: string; subjectId?: string; moduleId?: string; moduleVersion?: string; configuration?: Record<string, unknown> };
type WorkflowStepUpdate = { name: string; capabilityId?: string; subjectId?: string; distributionMode?:DistributionMode; configuration?: Record<string, unknown> };
type WorkflowModule = { moduleId: string; moduleVersion: string; name: string; description: string; stepType: 'api' | 'script' | 'module'; searchTerms: string[]; configurationSchema: { fields: { key: string; label: string; type: 'select' | 'url' | 'textarea' | 'code' | 'number' | 'text' | 'secret'; required?: boolean; default?: string | number; options?: string[]; placeholder?: string; min?: number; max?: number }[] }; publisher: string };
type ModuleInstallation=WorkflowModule&{enabled:boolean;allowedHosts:string[];allowSecrets:boolean;publisherTrusted:boolean};
type WorkspaceSecret = { id:string; name:string; secretType:'api_token'|'password'|'signing_key'|'other'; version:number; enabled:boolean; createdAt:string; updatedAt:string };
type ServiceScope='work:read'|'work:write';
type ServiceAccount={id:string;actorId:string;name:string;tokenPrefix:string;scopes:ServiceScope[];createdAt:string;lastUsedAt:string|null;revokedAt:string|null};
type ServiceCredential={serviceAccount:ServiceAccount;token:string};
type RemovedBranch={id:string;title:string;parentId:string;removedRevision:number;removedAt:string;nodeCount:number};
const SecretsContext=createContext<WorkspaceSecret[]>([]);
type TreeEditingContextValue={nodes:WorkNode[];busy:boolean;cutNodeId:string|null;dragNodeId:string|null;setCutNodeId:(id:string|null)=>void;setDragNodeId:(id:string|null)=>void;addChild:(id:string)=>void;focusNode:(id:string)=>void;canMove:(nodeId:string,parentId:string)=>boolean;moveNode:(nodeId:string,parentId:string)=>Promise<void>;removeNode:(node:WorkNode)=>Promise<void>};
const TreeEditingContext=createContext<TreeEditingContextValue|null>(null);
type WorkAction = 'claim' | 'submit' | 'close' | 'reopen' | 'retry' | 'cancel';
type PerformerMatch=ActorSummary&{confirmedRequirements:number;requirementCount:number;matchQuality:'verified'|'partially_verified'|'self_reported'};
type WorkCircle = { requester: ActorSummary; performer: ActorSummary | null; currentActor: ActorSummary; requirements: Capability[]; knowledgeRequirements: Pick<KnowledgeSubject, 'id' | 'name' | 'subjectType'>[]; openDescendants: DescendantBlocker[]; workflowSteps: WorkflowStep[]; matchingPerformers: PerformerMatch[]; permissions: { canClaim: boolean; canSubmit: boolean; canClose: boolean; canReturn: boolean; canReopen: boolean; canRetry: boolean; canCancel: boolean } };
type PriorityCandidate = { rootId: string; rootTitle: string; taskId: string; taskTitle: string; requester: ActorSummary };
type PriorityConflict = { left: PriorityCandidate; right: PriorityCandidate; affectedActors: ActorSummary[]; chosenRootId: string | null; decisionBasis: 'goal' | 'requester' | 'unknown' | null; decidedAt: string | null };
type PriorityInbox = { conflicts: PriorityConflict[]; rootScores: Record<string, number> };
type CriticalitySignal = { id: string; workNodeId: string; workNodeTitle: string; reason: string; criticalUntil: string; createdAt: string; affectedNodeIds: string[] };
type ActivityItem = { id: string; eventType: string; summary: string; detail: string; actorName: string; occurredAt: string; workspaceRevision: number | null; nodeId: string | null; nodeTitle:string; canPreview: boolean };
type MyWorkItem = { nodeId: string; nodeTitle: string; rootId: string; rootTitle: string; stepId: string; stepName: string; stepPosition: number; stepStatus: 'ready' | 'assigned' | 'active' | 'review'; requester: string; criticalUntil: string | null };
type BidRiskAssessment={adjustedDurationMinutes:number;expectedDurationMinutes:number;uncertaintyBufferMinutes:number};
type StepBid={id:string;workflowStepId:string;actor:ActorSummary;promisedDurationMinutes:number;status:'active'|'withdrawn'|'won'|'lost';submittedAt:string;updatedAt:string;withdrawnAt:string|null;estimation:EstimationStats;riskAssessment?:BidRiskAssessment};
type ExchangeOffer={nodeId:string;nodeTitle:string;rootId:string;rootTitle:string;stepId:string;stepName:string;stepPosition:number;requester:string;requirements:string[];knowledgeRequirements:string[];activeBidCount:number;myBid:StepBid|null};
type PersonalRank = { rank: number; stepStatus: MyWorkItem['stepStatus']; stepName: string };
type Diagnostic = { id:string; kind:'missing_requester'|'missing_outcome'|'wide_branch'|'agreed_duration_elapsed'|'requester_review_waiting'|'blocked_work_stagnant'|'no_eligible_performer'|'open_descendants'|'uncovered_knowledge'|'concentrated_knowledge'; severity:'warning'|'error'; title:string; explanation:string; nodeId:string; nodeTitle:string; subjectId:string; subjectName:string; evidence:string[]; relatedNodeIds:string[] };
type DiagnosticSettings = { wideBranchChildren:number;requesterReviewDays:number;blockedWorkDays:number };
type OnboardingStep={id:string;title:string;description:string;complete:boolean;action?:string;onAction?:()=>void};

function orderPersonalWork(items:MyWorkItem[],rootScores:Record<string,number>,criticalNodeIds:Set<string>){
  const weight=(status:MyWorkItem['stepStatus'])=>status==='active'?4:status==='assigned'?3:status==='review'?2:1;
  return [...items].sort((a,b)=>weight(b.stepStatus)-weight(a.stepStatus)||Number(Boolean(b.criticalUntil||criticalNodeIds.has(b.nodeId)))-Number(Boolean(a.criticalUntil||criticalNodeIds.has(a.nodeId)))||(rootScores[b.rootId]??0)-(rootScores[a.rootId]??0)||a.nodeTitle.localeCompare(b.nodeTitle));
}

async function api<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...options,
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...options?.headers },
  });
  const body = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(body.error ?? 'The request failed');
  return body as T;
}

function App() {
  const deepLink=useRef({workspace:new URLSearchParams(window.location.search).get('workspace')??'',node:new URLSearchParams(window.location.search).get('node')??''});
  const [readiness, setReadiness] = useState<Readiness>('checking');
  const [auth, setAuth] = useState<AuthStatus | null>(null);
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [workspaceId, setWorkspaceId] = useState('');
  const [nodes, setNodes] = useState<WorkNode[]>([]);
  const [history, setHistory] = useState<ChangeEvent[]>([]);
  const [directory, setDirectory] = useState<Directory>({ actors: [], capabilities: [], currentWorkspaceRole: 'member' });
  const [circle, setCircle] = useState<WorkCircle | null>(null);
  const [profile, setProfile] = useState<MyProfile | null>(null);
  const [knowledgeSubjects, setKnowledgeSubjects] = useState<KnowledgeSubject[]>([]);
  const [workflowModules, setWorkflowModules] = useState<WorkflowModule[]>([]);
  const [secrets,setSecrets]=useState<WorkspaceSecret[]>([]);
  const [serviceAccounts,setServiceAccounts]=useState<ServiceAccount[]>([]);
  const [issuedCredential,setIssuedCredential]=useState<ServiceCredential|null>(null);
  const [moduleInstallations,setModuleInstallations]=useState<ModuleInstallation[]>([]);
  const [removedBranches,setRemovedBranches]=useState<RemovedBranch[]>([]);
  const [cutNodeId,setCutNodeId]=useState<string|null>(null);
  const [dragNodeId,setDragNodeId]=useState<string|null>(null);
  const [branchCollapse,setBranchCollapse]=useState<Record<string,boolean>>({});
  const [treeSearchOpen,setTreeSearchOpen]=useState(false);
  const [treeSearchQuery,setTreeSearchQuery]=useState('');
  const [focusedBranchId,setFocusedBranchId]=useState<string|null>(null);
  const [priorities, setPriorities] = useState<PriorityInbox>({ conflicts: [], rootScores: {} });
  const [criticality, setCriticality] = useState<CriticalitySignal[]>([]);
  const [activity, setActivity] = useState<ActivityItem[]>([]);
  const [myWork,setMyWork]=useState<MyWorkItem[]>([]);
  const [exchangeOffers,setExchangeOffers]=useState<ExchangeOffer[]>([]);
  const [diagnostics,setDiagnostics]=useState<Diagnostic[]>([]);
  const [diagnosticSettings,setDiagnosticSettings]=useState<DiagnosticSettings>({wideBranchChildren:8,requesterReviewDays:7,blockedWorkDays:14});
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [dialog, setDialog] = useState<'workspace' | 'root' | 'child' | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [view, setView] = useState<'my-work' | 'exchange' | 'structure' | 'history' | 'directory' | 'profile' | 'priorities' | 'diagnostics' | 'secrets' | 'modules' | 'api-access'>('structure');
  const [previewRevision, setPreviewRevision] = useState<number | null>(null);
  const [previewNodes, setPreviewNodes] = useState<WorkNode[]>([]);
  const [onboardingDismissed,setOnboardingDismissed]=useState(false);
  const [firstWorkspacePrompt,setFirstWorkspacePrompt]=useState(false);

  const selected = nodes.find((node) => node.id === selectedId) ?? null;
  const currentWorkspace = workspaces.find((workspace) => workspace.id === workspaceId);
  const accountId=auth?.account?.id;
  const canAdminister = directory.currentWorkspaceRole === 'owner' || directory.currentWorkspaceRole === 'admin';
  const isAdminView = view === 'directory' || view === 'priorities' || view === 'diagnostics' || view === 'modules' || view === 'secrets' || view === 'api-access';
  const roots = useMemo(() => nodes.filter((node) => node.parentId === null), [nodes]);
  const structureNodes = previewRevision === null ? nodes : previewNodes;
  const structureRoots = structureNodes.filter((node) => node.parentId === null);
  const structureSelected = structureNodes.find((node) => node.id === selectedId) ?? structureNodes[0] ?? null;
  const previewChange = previewRevision===null||!structureSelected?null:history.find((event)=>event.workspaceRevision===previewRevision&&event.entityId===structureSelected.id)??null;
  const previewActivity = previewRevision===null||!structureSelected?null:activity.find((item)=>item.workspaceRevision===previewRevision&&item.nodeId===structureSelected.id)??null;
  const historyPoints=useMemo(()=>{const points=new Map<number,{revision:number;nodeId:string}>();for(const item of activity){if(item.canPreview&&item.workspaceRevision&&item.nodeId&&!points.has(item.workspaceRevision))points.set(item.workspaceRevision,{revision:item.workspaceRevision,nodeId:item.nodeId})}return [...points.values()].sort((a,b)=>a.revision-b.revision)},[activity]);
  const historyPosition=previewRevision===null?-1:historyPoints.findIndex((item)=>item.revision===previewRevision);
  const focusedBranch=structureNodes.find((node)=>node.id===focusedBranchId)??null;
  const visibleStructureRoots=focusedBranch?[focusedBranch]:structureRoots;
  const selectedPath=useMemo(()=>{const path:WorkNode[]=[];let cursor:WorkNode|null=structureSelected;while(cursor){path.unshift(cursor);const parentId:string|null=cursor.parentId;cursor=parentId?structureNodes.find((node)=>node.id===parentId)??null:null}return path},[structureSelected,structureNodes]);
  const searchResults=treeSearchQuery.trim()?nodes.filter((node)=>`${node.title} ${node.desiredOutcome}`.toLowerCase().includes(treeSearchQuery.trim().toLowerCase())).slice(0,8):[];
  const criticalNodeIds = useMemo(() => new Set(criticality.flatMap((signal) => signal.affectedNodeIds)), [criticality]);
  const personalRanks=useMemo(()=>new Map(orderPersonalWork(myWork,priorities.rootScores,criticalNodeIds).map((item,index)=>[item.nodeId,{rank:index+1,stepStatus:item.stepStatus,stepName:item.stepName} satisfies PersonalRank])),[myWork,priorities.rootScores,criticalNodeIds]);
  const diagnosticsByNode=useMemo(()=>{const result=new Map<string,Diagnostic[]>();for(const item of diagnostics)if(item.nodeId)result.set(item.nodeId,[...(result.get(item.nodeId)??[]),item]);return result},[diagnostics]);
  const onboardingSteps:OnboardingStep[]=[
    {id:'goal',title:'Set the outcome',description:'Your root goal keeps every task connected to its purpose.',complete:roots.length>0},
    {id:'branch',title:'Break the goal into work',description:'Create the first child task and start a visible branch.',complete:nodes.some((node)=>node.parentId!==null),action:'Add first task',onAction:()=>{const root=roots[0];if(root){setSelectedId(root.id);setDialog('child')}}},
    {id:'capability',title:'Describe a role or skill',description:'Use capabilities instead of assigning work by a person’s name.',complete:directory.capabilities.length>0,action:'Set up directory',onAction:canAdminister?()=>setView('directory'):undefined},
    {id:'profile',title:'Add your first skill',description:'Tell the system what kind of work you can take.',complete:Boolean(profile?.claims.some((claim)=>claim.capabilityType==='skill')),action:'Open my profile',onAction:directory.capabilities.some((item)=>item.capabilityType==='skill')?()=>setView('profile'):undefined},
    {id:'participant',title:'Invite a collaborator',description:'Bring another person into this workspace when you are ready.',complete:directory.actors.filter((actor)=>actor.hasAccount).length>1,action:'Invite someone',onAction:canAdminister?()=>setView('directory'):undefined},
  ];
  const onboardingComplete=onboardingSteps.every((step)=>step.complete);
  function canMoveBranch(nodeId:string,parentId:string){
    const source=nodes.find((node)=>node.id===nodeId);const parent=nodes.find((node)=>node.id===parentId);
    if(!source?.parentId||source.lifecycleStatus!=='planned'||!parent||parent.lifecycleStatus!=='planned'||nodeId===parentId)return false;
    let cursor:WorkNode|undefined=parent;while(cursor){if(cursor.id===nodeId)return false;cursor=cursor.parentId?nodes.find((node)=>node.id===cursor?.parentId):undefined}
    return source.parentId!==parentId;
  }

  useEffect(() => {
    Promise.all([api<{ status: string }>('/api/v1/ready'), api<AuthStatus>('/api/v1/auth/status')])
      .then(([, status]) => {
        setReadiness('ready');
        setAuth(status);
        if (status.authenticated) loadWorkspaces();
      })
      .catch((cause: Error) => {
        setReadiness('not-ready');
        setError(cause.message);
      });
  }, []);

  async function loadWorkspaces() {
    const items = await api<Workspace[]>('/api/v1/workspaces');
    setWorkspaces(items);
    const requested=deepLink.current.workspace;
    setWorkspaceId(items.some((item)=>item.id===requested)?requested:items[0]?.id??'');
    return items;
  }

  async function authenticated(account: Account, newAccount=false) {
    setAuth((current)=>({ setupRequired: false, registrationEnabled: current?.registrationEnabled ?? false,passwordResetEnabled:current?.passwordResetEnabled??false, authenticated: true, account }));
    const items=await loadWorkspaces();
    if(newAccount&&items.length===0){setFirstWorkspacePrompt(true);setDialog('workspace')}
  }

  async function logout() {
    await api<void>('/api/v1/auth/logout', { method: 'POST', body: '{}' });
    setAuth((current)=>({ setupRequired: false, registrationEnabled: current?.registrationEnabled ?? false,passwordResetEnabled:current?.passwordResetEnabled??false, authenticated: false, account: null }));
    setWorkspaces([]); setWorkspaceId(''); setNodes([]);
  }

  useEffect(() => {
    if (!workspaceId) {
      setNodes([]);
      setHistory([]);
      setDirectory({ actors: [], capabilities: [], currentWorkspaceRole: 'member' });
      setProfile(null);
      setKnowledgeSubjects([]);
      setWorkflowModules([]);
      setSecrets([]);
      setServiceAccounts([]);
      setIssuedCredential(null);
      setModuleInstallations([]);
      setRemovedBranches([]);
      setPriorities({ conflicts: [], rootScores: {} });
      setCriticality([]);
      setActivity([]);
      setMyWork([]);
      setExchangeOffers([]);
      setDiagnostics([]);
      return;
    }
    setPreviewRevision(null);
    setPreviewNodes([]);
    Promise.all([
      api<WorkNode[]>(`/api/v1/workspaces/${workspaceId}/nodes`),
      api<ChangeEvent[]>(`/api/v1/workspaces/${workspaceId}/history`),
      api<Directory>(`/api/v1/workspaces/${workspaceId}/directory`),
      api<MyProfile>(`/api/v1/workspaces/${workspaceId}/me`),
      api<KnowledgeSubject[]>(`/api/v1/workspaces/${workspaceId}/knowledge-subjects`),
      api<PriorityInbox>(`/api/v1/workspaces/${workspaceId}/priorities`),
      api<CriticalitySignal[]>(`/api/v1/workspaces/${workspaceId}/criticality`),
      api<ActivityItem[]>(`/api/v1/workspaces/${workspaceId}/activity`),
      api<MyWorkItem[]>(`/api/v1/workspaces/${workspaceId}/my-work`),
      api<ExchangeOffer[]>(`/api/v1/workspaces/${workspaceId}/exchange`),
      api<WorkflowModule[]>(`/api/v1/workspaces/${workspaceId}/workflow-modules`),
      api<WorkspaceSecret[]>(`/api/v1/workspaces/${workspaceId}/secrets`),
      api<RemovedBranch[]>(`/api/v1/workspaces/${workspaceId}/removed-branches`),
      api<Diagnostic[]>(`/api/v1/workspaces/${workspaceId}/diagnostics`),
      api<DiagnosticSettings>(`/api/v1/workspaces/${workspaceId}/diagnostic-settings`),
    ])
      .then(([items, events, directoryData, profileData, subjects, priorityData, criticalityData, activityData,personalWork,offers,modules,secretItems,removedItems,diagnosticItems,diagnosticSettingsData]) => {
        setNodes(items);
        setHistory(events);
        setDirectory(directoryData);
        setProfile(profileData);
        setKnowledgeSubjects(subjects);
        setPriorities(priorityData);
        setCriticality(criticalityData);
        setActivity(activityData);
        setMyWork(personalWork);
        setExchangeOffers(offers);
        setWorkflowModules(modules);
        setSecrets(secretItems);
        setRemovedBranches(removedItems);
        setDiagnostics(diagnosticItems);
        setDiagnosticSettings(diagnosticSettingsData);
        const requestedNode=deepLink.current.node;
        if(requestedNode&&items.some((item)=>item.id===requestedNode)){
          const ancestors:string[]=[];let cursor=items.find((item)=>item.id===requestedNode);
          while(cursor?.parentId){ancestors.push(cursor.parentId);cursor=items.find((item)=>item.id===cursor?.parentId)}
          setBranchCollapse((current)=>{const next={...current};for(const id of ancestors)next[id]=false;return next});
          setFocusedBranchId(null);setView('structure');setSelectedId(requestedNode);deepLink.current.node='';
        }else setSelectedId((current) => current && items.some((item) => item.id === current) ? current : items[0]?.id ?? null);
      })
      .catch((cause: Error) => setError(cause.message));
  }, [workspaceId]);

  useEffect(()=>{
    if(!workspaceId||!accountId)return;
    const key=`work-graph:collapsed:${accountId}:${workspaceId}`;
    try{setBranchCollapse(JSON.parse(window.localStorage.getItem(key)??'{}'))}catch{setBranchCollapse({})}
  },[workspaceId,accountId]);

  useEffect(()=>{
    if(!workspaceId||!accountId){setOnboardingDismissed(false);return}
    setOnboardingDismissed(window.localStorage.getItem(`work-graph:onboarding-dismissed:${accountId}:${workspaceId}`)==='true');
  },[workspaceId,accountId]);

  function dismissOnboarding(){if(workspaceId&&accountId)window.localStorage.setItem(`work-graph:onboarding-dismissed:${accountId}:${workspaceId}`,'true');setOnboardingDismissed(true);}

  function toggleBranch(nodeId:string,collapsed:boolean){
    setBranchCollapse((current)=>{
      const next={...current,[nodeId]:collapsed};
      if(workspaceId&&accountId)window.localStorage.setItem(`work-graph:collapsed:${accountId}:${workspaceId}`,JSON.stringify(next));
      return next;
    });
  }

  function revealTreeNode(nodeId:string){
    const ancestors:string[]=[];let cursor=nodes.find((node)=>node.id===nodeId);
    while(cursor?.parentId){ancestors.push(cursor.parentId);cursor=nodes.find((node)=>node.id===cursor?.parentId)}
    setBranchCollapse((current)=>{const next={...current};for(const id of ancestors)next[id]=false;if(workspaceId&&accountId)window.localStorage.setItem(`work-graph:collapsed:${accountId}:${workspaceId}`,JSON.stringify(next));return next});
    setFocusedBranchId(null);setSelectedId(nodeId);setTreeSearchOpen(false);setTreeSearchQuery('');
  }

  function focusTreeNode(nodeId:string){setFocusedBranchId(nodeId);setSelectedId(nodeId);setBranchCollapse((current)=>({...current,[nodeId]:false}));}

  useEffect(()=>{
    if(view==='secrets'&&workspaceId&&canAdminister)api<WorkspaceSecret[]>(`/api/v1/workspaces/${workspaceId}/secrets`).then(setSecrets).catch((cause:Error)=>setError(cause.message));
  },[view,workspaceId,canAdminister]);
  useEffect(()=>{
    if(view==='modules'&&workspaceId&&canAdminister)api<ModuleInstallation[]>(`/api/v1/workspaces/${workspaceId}/module-installations`).then(setModuleInstallations).catch((cause:Error)=>setError(cause.message));
  },[view,workspaceId,canAdminister]);
  useEffect(()=>{
    if(view==='api-access'&&workspaceId&&canAdminister)api<ServiceAccount[]>(`/api/v1/workspaces/${workspaceId}/service-accounts`).then(setServiceAccounts).catch((cause:Error)=>setError(cause.message));
  },[view,workspaceId,canAdminister]);

  useEffect(() => {
    if (!workspaceId || !selectedId || previewRevision !== null) {
      setCircle(null);
      return;
    }
    api<WorkCircle>(`/api/v1/workspaces/${workspaceId}/nodes/${selectedId}/circle`).then(setCircle).catch((cause: Error) => setError(cause.message));
  }, [workspaceId, selectedId, previewRevision]);

  useEffect(()=>{
    if(view!=='structure'||previewRevision!==null||!selectedId)return;
    const frame=window.requestAnimationFrame(()=>document.querySelector<HTMLElement>(`[data-node-id="${CSS.escape(selectedId)}"]`)?.scrollIntoView({behavior:'smooth',block:'center',inline:'center'}));
    return ()=>window.cancelAnimationFrame(frame);
  },[view,selectedId,previewRevision]);

  useEffect(()=>{
    function handleTreeClipboard(event:KeyboardEvent){
      if(view!=='structure'||previewRevision!==null||busy)return;
      const target=event.target as HTMLElement|null;if(target?.matches('input,textarea,select,[contenteditable="true"]'))return;
      if((event.ctrlKey||event.metaKey)&&event.key.toLowerCase()==='x'&&selectedId&&nodes.find((node)=>node.id===selectedId)?.parentId&&nodes.find((node)=>node.id===selectedId)?.lifecycleStatus==='planned'){event.preventDefault();setCutNodeId(selectedId);return}
      if((event.ctrlKey||event.metaKey)&&event.key.toLowerCase()==='v'&&cutNodeId&&selectedId&&canMoveBranch(cutNodeId,selectedId)){event.preventDefault();void perform(async()=>{await moveTreeNode(cutNodeId,selectedId);setCutNodeId(null)})}
      if(event.key==='Escape'&&cutNodeId)setCutNodeId(null);
    }
    window.addEventListener('keydown',handleTreeClipboard);return()=>window.removeEventListener('keydown',handleTreeClipboard);
  },[view,previewRevision,busy,selectedId,cutNodeId,nodes]);

  useEffect(()=>{function openTreeSearch(event:KeyboardEvent){if((event.ctrlKey||event.metaKey)&&event.key.toLowerCase()==='k'){event.preventDefault();setView('structure');setTreeSearchOpen(true)}}window.addEventListener('keydown',openTreeSearch);return()=>window.removeEventListener('keydown',openTreeSearch)},[]);

  useEffect(()=>{function createChildFromTree(event:KeyboardEvent){const target=event.target as HTMLElement|null;if(view!=='structure'||previewRevision!==null||busy||event.ctrlKey||event.metaKey||event.altKey||event.key.toLowerCase()!=='n'||target?.matches('input,textarea,select,[contenteditable="true"]')||!selectedId)return;event.preventDefault();setDialog('child')}window.addEventListener('keydown',createChildFromTree);return()=>window.removeEventListener('keydown',createChildFromTree)},[view,previewRevision,busy,selectedId]);

  async function createWorkspace(name: string, rootTitle: string) {
    const result = await api<{ workspace: Workspace; root: WorkNode }>('/api/v1/workspaces', {
      method: 'POST', body: JSON.stringify({ name, rootTitle }),
    });
    setWorkspaces((items) => [...items, result.workspace]);
    setWorkspaceId(result.workspace.id);
    setNodes([result.root]);
    setHistory(await api<ChangeEvent[]>(`/api/v1/workspaces/${result.workspace.id}/history`));
    setSelectedId(result.root.id);
    setFirstWorkspacePrompt(false);
  }

  async function createNode(title: string, desiredOutcome: string, parentId: string | null) {
    const node = await api<WorkNode>(`/api/v1/workspaces/${workspaceId}/nodes`, {
      method: 'POST', body: JSON.stringify({ title, desiredOutcome, parentId }),
    });
    setNodes((items) => [...items, node]);
    setHistory(await api<ChangeEvent[]>(`/api/v1/workspaces/${workspaceId}/history`));
    await refreshActivity();
    await refreshDiagnostics();
    setSelectedId(node.id);
  }

  async function saveNode(input: Pick<WorkNode, 'title' | 'desiredOutcome' | 'lifecycleStatus'>) {
    if (!selected) return;
    const updated = await api<WorkNode>(`/api/v1/workspaces/${workspaceId}/nodes/${selected.id}`, {
      method: 'PATCH', body: JSON.stringify(input),
    });
    setNodes((items) => items.map((item) => item.id === updated.id ? updated : item));
    setHistory(await api<ChangeEvent[]>(`/api/v1/workspaces/${workspaceId}/history`));
    await refreshActivity();
    await refreshDiagnostics();
  }

  async function previewAtRevision(revision: number, entityId: string) {
    setBusy(true);
    setError('');
    try {
      const snapshot = await api<{ revision: number; currentRevision: number; nodes: WorkNode[] }>(`/api/v1/workspaces/${workspaceId}/revisions/${revision}/nodes`);
      setPreviewRevision(snapshot.revision);
      setPreviewNodes(snapshot.nodes);
      setSelectedId(snapshot.nodes.some((node) => node.id === entityId) ? entityId : snapshot.nodes[0]?.id ?? null);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'The revision could not be loaded');
    } finally {
      setBusy(false);
    }
  }

  function returnToCurrent() {
    setPreviewRevision(null);
    setPreviewNodes([]);
    setSelectedId((current) => current && nodes.some((node) => node.id === current) ? current : nodes[0]?.id ?? null);
  }

  function openHistory() {
    setView('history');
    const latest = activity.find((item) => item.canPreview && item.workspaceRevision && item.nodeId);
    if (latest?.workspaceRevision && latest.nodeId) void previewAtRevision(latest.workspaceRevision, latest.nodeId);
  }

  function openCurrentTree() {
    returnToCurrent();
    setView('structure');
  }

  function moveHistory(direction:-1|1) {
    const target=historyPoints[historyPosition+direction];
    if(target)void previewAtRevision(target.revision,target.nodeId);
  }

  async function refreshDirectory() {
    setDirectory(await api<Directory>(`/api/v1/workspaces/${workspaceId}/directory`));
  }
  async function updateWorkDistribution(mode:Workspace['workDistributionMode']){const updated=await api<Workspace>(`/api/v1/workspaces/${workspaceId}/distribution`,{method:'PATCH',body:JSON.stringify({mode})});setWorkspaces((items)=>items.map((item)=>item.id===updated.id?updated:item));if(mode==='simple'&&view==='exchange')setView('structure');await Promise.all([refreshExchange(),refreshActivity()]);}
  async function refreshActivity() { setActivity(await api<ActivityItem[]>(`/api/v1/workspaces/${workspaceId}/activity`)); }
  async function refreshMyWork() { setMyWork(await api<MyWorkItem[]>(`/api/v1/workspaces/${workspaceId}/my-work`)); }
  async function refreshExchange(){setExchangeOffers(await api<ExchangeOffer[]>(`/api/v1/workspaces/${workspaceId}/exchange`));}
  async function submitBid(offer:ExchangeOffer,promisedDurationMinutes:number){await api(`/api/v1/workspaces/${workspaceId}/nodes/${offer.nodeId}/workflow-steps/${offer.stepId}/bids/me`,{method:'PUT',body:JSON.stringify({promisedDurationMinutes})});await Promise.all([refreshExchange(),refreshActivity()]);}
  async function withdrawBid(offer:ExchangeOffer){await api(`/api/v1/workspaces/${workspaceId}/nodes/${offer.nodeId}/workflow-steps/${offer.stepId}/bids/me`,{method:'DELETE'});await Promise.all([refreshExchange(),refreshActivity()]);}
  async function refreshDiagnostics(){setDiagnostics(await api<Diagnostic[]>(`/api/v1/workspaces/${workspaceId}/diagnostics`));}
  async function updateDiagnosticSettings(input:DiagnosticSettings){const result=await api<DiagnosticSettings>(`/api/v1/workspaces/${workspaceId}/diagnostic-settings`,{method:'PATCH',body:JSON.stringify(input)});setDiagnosticSettings(result);await refreshDiagnostics()}
  async function refreshSecrets(){setSecrets(await api<WorkspaceSecret[]>(`/api/v1/workspaces/${workspaceId}/secrets`));}
  async function createSecret(name:string,secretType:WorkspaceSecret['secretType'],value:string){await api(`/api/v1/workspaces/${workspaceId}/secrets`,{method:'POST',body:JSON.stringify({name,secretType,value})});await refreshSecrets();}
  async function rotateSecret(secretId:string,value:string){await api(`/api/v1/workspaces/${workspaceId}/secrets/${secretId}/rotate`,{method:'POST',body:JSON.stringify({value})});await refreshSecrets();}
  async function disableSecret(secretId:string){await api(`/api/v1/workspaces/${workspaceId}/secrets/${secretId}`,{method:'DELETE'});await refreshSecrets();}
  async function refreshServiceAccounts(){setServiceAccounts(await api<ServiceAccount[]>(`/api/v1/workspaces/${workspaceId}/service-accounts`));}
  async function createServiceAccount(name:string,scopes:ServiceScope[]){const credential=await api<ServiceCredential>(`/api/v1/workspaces/${workspaceId}/service-accounts`,{method:'POST',body:JSON.stringify({name,scopes})});setIssuedCredential(credential);await refreshServiceAccounts();}
  async function rotateServiceAccount(id:string){const credential=await api<ServiceCredential>(`/api/v1/workspaces/${workspaceId}/service-accounts/${id}/rotate`,{method:'POST',body:'{}'});setIssuedCredential(credential);await refreshServiceAccounts();}
  async function revokeServiceAccount(id:string){await api(`/api/v1/workspaces/${workspaceId}/service-accounts/${id}`,{method:'DELETE'});await refreshServiceAccounts();}
  async function updateModulePolicy(item:ModuleInstallation,policy:Pick<ModuleInstallation,'enabled'|'allowedHosts'|'allowSecrets'|'publisherTrusted'>){await api(`/api/v1/workspaces/${workspaceId}/module-installations/${encodeURIComponent(item.moduleId)}/${encodeURIComponent(item.moduleVersion)}`,{method:'PATCH',body:JSON.stringify(policy)});setModuleInstallations(await api<ModuleInstallation[]>(`/api/v1/workspaces/${workspaceId}/module-installations`));setWorkflowModules(await api<WorkflowModule[]>(`/api/v1/workspaces/${workspaceId}/workflow-modules`));}
  async function reloadTree(){const [items,events,activityItems,personalWork,removedItems,diagnosticItems]=await Promise.all([api<WorkNode[]>(`/api/v1/workspaces/${workspaceId}/nodes`),api<ChangeEvent[]>(`/api/v1/workspaces/${workspaceId}/history`),api<ActivityItem[]>(`/api/v1/workspaces/${workspaceId}/activity`),api<MyWorkItem[]>(`/api/v1/workspaces/${workspaceId}/my-work`),api<RemovedBranch[]>(`/api/v1/workspaces/${workspaceId}/removed-branches`),api<Diagnostic[]>(`/api/v1/workspaces/${workspaceId}/diagnostics`)]);setNodes(items);setHistory(events);setActivity(activityItems);setMyWork(personalWork);setRemovedBranches(removedItems);setDiagnostics(diagnosticItems);return items;}
  async function moveTreeNode(nodeId:string,parentId:string){await api(`/api/v1/workspaces/${workspaceId}/nodes/${nodeId}/move`,{method:'POST',body:JSON.stringify({parentId})});await reloadTree();setCircle(await api<WorkCircle>(`/api/v1/workspaces/${workspaceId}/nodes/${nodeId}/circle`));}
  async function removeTreeNode(node:WorkNode){await api(`/api/v1/workspaces/${workspaceId}/nodes/${node.id}`,{method:'DELETE'});const items=await reloadTree();setSelectedId(node.parentId&&items.some((item)=>item.id===node.parentId)?node.parentId:items[0]?.id??null);}
  async function restoreTreeNode(branch:RemovedBranch){const restored=await api<WorkNode>(`/api/v1/workspaces/${workspaceId}/removed-branches/${branch.id}/restore`,{method:'POST',body:'{}'});await reloadTree();setSelectedId(restored.id);}

  async function createActor(displayName: string, actorType: Actor['actorType']) {
    await api(`/api/v1/workspaces/${workspaceId}/actors`, { method: 'POST', body: JSON.stringify({ displayName, actorType }) });
    await refreshDirectory();
  }

  async function createCapability(name: string, capabilityType: Capability['capabilityType']) {
    await api(`/api/v1/workspaces/${workspaceId}/capabilities`, { method: 'POST', body: JSON.stringify({ name, capabilityType }) });
    await refreshDirectory();
  }

  async function assignCapability(actorId: string, capabilityId: string) {
    await api(`/api/v1/workspaces/${workspaceId}/actors/${actorId}/capabilities`, { method: 'POST', body: JSON.stringify({ capabilityId, claimSource: 'admin' }) });
    await refreshDirectory(); await refreshDiagnostics();
  }

  async function refreshKnowledge() { setKnowledgeSubjects(await api<KnowledgeSubject[]>(`/api/v1/workspaces/${workspaceId}/knowledge-subjects`)); }
  async function createKnowledgeSubject(name: string, subjectType: KnowledgeSubject['subjectType']) { await api(`/api/v1/workspaces/${workspaceId}/knowledge-subjects`, { method: 'POST', body: JSON.stringify({ name, subjectType }) }); await refreshKnowledge(); }
  async function assignKnowledge(actorId: string, subjectId: string) { await api(`/api/v1/workspaces/${workspaceId}/actors/${actorId}/knowledge`, { method: 'POST', body: JSON.stringify({ subjectId }) }); await refreshKnowledge(); await refreshDiagnostics(); }
  async function removeCapability(actorId:string,capabilityId:string){await api(`/api/v1/workspaces/${workspaceId}/actors/${actorId}/capabilities/${capabilityId}`,{method:'DELETE'});await refreshDirectory();await refreshProfile();await refreshDiagnostics()}
  async function updateMemberRole(actorId:string,workspaceRole:'admin'|'member'){await api(`/api/v1/workspaces/${workspaceId}/actors/${actorId}/membership`,{method:'PATCH',body:JSON.stringify({workspaceRole})});await refreshDirectory();await refreshActivity()}
  async function removeMember(actorId:string){await api(`/api/v1/workspaces/${workspaceId}/actors/${actorId}/membership`,{method:'DELETE'});await refreshDirectory();await refreshKnowledge();await refreshDiagnostics();await refreshActivity()}
  async function removeKnowledge(actorId:string,subjectId:string){await api(`/api/v1/workspaces/${workspaceId}/actors/${actorId}/knowledge/${subjectId}`,{method:'DELETE'});await refreshKnowledge();await refreshProfile();await refreshDiagnostics()}
  async function decidePriority(conflict: PriorityConflict, chosenRootId: string | null, decisionBasis: 'goal' | 'requester' | 'unknown') { await api(`/api/v1/workspaces/${workspaceId}/priority-decisions`, { method: 'POST', body: JSON.stringify({ leftRootId: conflict.left.rootId, rightRootId: conflict.right.rootId, chosenRootId, decisionBasis }) }); setPriorities(await api<PriorityInbox>(`/api/v1/workspaces/${workspaceId}/priorities`)); await refreshActivity(); }
  async function refreshCriticality() { setCriticality(await api<CriticalitySignal[]>(`/api/v1/workspaces/${workspaceId}/criticality`)); }
  async function markCritical(reason: string, criticalUntil: string) { if (!selectedId) return; await api(`/api/v1/workspaces/${workspaceId}/nodes/${selectedId}/criticality`, { method: 'POST', body: JSON.stringify({ reason, criticalUntil: new Date(criticalUntil).toISOString() }) }); await refreshCriticality(); await refreshActivity(); }
  async function revokeCritical(signalId: string) { await api(`/api/v1/workspaces/${workspaceId}/criticality/${signalId}`, { method: 'DELETE' }); await refreshCriticality(); await refreshActivity(); }

  async function addRequirement(capabilityId: string) {
    if (!selectedId) return;
    await api(`/api/v1/workspaces/${workspaceId}/nodes/${selectedId}/requirements`, { method: 'POST', body: JSON.stringify({ capabilityId }) });
    setCircle(await api<WorkCircle>(`/api/v1/workspaces/${workspaceId}/nodes/${selectedId}/circle`));
    await refreshDiagnostics();
  }

  async function addKnowledgeRequirement(subjectId: string) {
    if (!selectedId) return;
    await api(`/api/v1/workspaces/${workspaceId}/nodes/${selectedId}/knowledge-requirements`, { method: 'POST', body: JSON.stringify({ subjectId }) });
    setCircle(await api<WorkCircle>(`/api/v1/workspaces/${workspaceId}/nodes/${selectedId}/circle`));
    await refreshDiagnostics();
  }

  async function addWorkflowStep(input: NewWorkflowStep) {
    if (!selectedId) return;
    await api(`/api/v1/workspaces/${workspaceId}/nodes/${selectedId}/workflow-steps`, { method: 'POST', body: JSON.stringify(input) });
    setCircle(await api<WorkCircle>(`/api/v1/workspaces/${workspaceId}/nodes/${selectedId}/circle`));
    await refreshActivity();
    await refreshDiagnostics();
  }

  async function updateWorkflowStep(stepId:string,input:WorkflowStepUpdate) { if(!selectedId)return; await api(`/api/v1/workspaces/${workspaceId}/nodes/${selectedId}/workflow-steps/${stepId}`,{method:'PATCH',body:JSON.stringify(input)}); setCircle(await api<WorkCircle>(`/api/v1/workspaces/${workspaceId}/nodes/${selectedId}/circle`)); await refreshActivity(); await refreshDiagnostics(); }
  async function moveWorkflowStep(stepId:string,direction:'earlier'|'later') { if(!selectedId)return; await api(`/api/v1/workspaces/${workspaceId}/nodes/${selectedId}/workflow-steps/${stepId}/move`,{method:'POST',body:JSON.stringify({direction})}); setCircle(await api<WorkCircle>(`/api/v1/workspaces/${workspaceId}/nodes/${selectedId}/circle`)); await refreshActivity(); }
  async function deleteWorkflowStep(stepId:string) { if(!selectedId)return; await api(`/api/v1/workspaces/${workspaceId}/nodes/${selectedId}/workflow-steps/${stepId}`,{method:'DELETE'}); setCircle(await api<WorkCircle>(`/api/v1/workspaces/${workspaceId}/nodes/${selectedId}/circle`)); await refreshActivity(); await refreshDiagnostics(); }
  async function selectWorkflowStepBid(stepId:string){if(!selectedId)return;await api(`/api/v1/workspaces/${workspaceId}/nodes/${selectedId}/workflow-steps/${stepId}/bids/select`,{method:'POST',body:'{}'});setCircle(await api<WorkCircle>(`/api/v1/workspaces/${workspaceId}/nodes/${selectedId}/circle`));await Promise.all([refreshExchange(),refreshMyWork(),refreshActivity()]);}

  async function refreshProfile() {
    setProfile(await api<MyProfile>(`/api/v1/workspaces/${workspaceId}/me`));
    await refreshDirectory();
  }

  async function addMySkill(capabilityId: string, level: ClaimEvidence['level'], note: string) {
    await api(`/api/v1/workspaces/${workspaceId}/me/skills`, { method: 'POST', body: JSON.stringify({ capabilityId, level, note }) });
    await refreshProfile();
  }

  async function removeMySkill(capabilityId: string) {
    await api(`/api/v1/workspaces/${workspaceId}/me/skills/${capabilityId}`, { method: 'DELETE' });
    await refreshProfile();
  }

  async function addMyKnowledge(subjectId:string,level:ClaimEvidence['level'],note:string){await api(`/api/v1/workspaces/${workspaceId}/me/knowledge`,{method:'POST',body:JSON.stringify({subjectId,level,note})});await refreshProfile()}
  async function removeMyKnowledge(subjectId:string){await api(`/api/v1/workspaces/${workspaceId}/me/knowledge/${subjectId}`,{method:'DELETE'});await refreshProfile()}

  async function performWorkAction(action: WorkAction) {
    if (!selectedId) return;
    await performWorkActionFor(selectedId,action);
  }

  async function performWorkActionFor(nodeId: string, action: WorkAction) {
    const updated = await api<WorkNode>(`/api/v1/workspaces/${workspaceId}/nodes/${nodeId}/actions/${action}`, { method: 'POST', body: '{}' });
    setNodes((items) => items.map((item) => item.id === updated.id ? updated : item));
    if(selectedId===nodeId)setCircle(await api<WorkCircle>(`/api/v1/workspaces/${workspaceId}/nodes/${nodeId}/circle`));
    setHistory(await api<ChangeEvent[]>(`/api/v1/workspaces/${workspaceId}/history`));
    await refreshMyWork();
    await refreshActivity();
    await refreshDiagnostics();
    if(action==='submit')await refreshProfile();
  }

  async function returnForRevision(stepId:string,reason:string){
    if(!selectedId)return;
    const updated=await api<WorkNode>(`/api/v1/workspaces/${workspaceId}/nodes/${selectedId}/actions/return`,{method:'POST',body:JSON.stringify({stepId,reason})});
    setNodes((items)=>items.map((item)=>item.id===updated.id?updated:item));
    setCircle(await api<WorkCircle>(`/api/v1/workspaces/${workspaceId}/nodes/${selectedId}/circle`));
    setHistory(await api<ChangeEvent[]>(`/api/v1/workspaces/${workspaceId}/history`));
    await refreshMyWork(); await refreshActivity(); await refreshDiagnostics();
  }

  async function perform(action: () => Promise<void>) {
    setBusy(true);
    setError('');
    try {
      await action();
      setDialog(null);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'The request failed');
    } finally {
      setBusy(false);
    }
  }

  if (readiness === 'checking' || auth === null) return <AuthShell><p className="muted">Connecting to Work Graph…</p></AuthShell>;
  if (readiness === 'not-ready') return <AuthShell><div className="error" role="alert">{error || 'Work Graph is unavailable.'}</div></AuthShell>;
  const invitationToken = new URLSearchParams(window.location.search).get('invite');
  const passwordResetToken = new URLSearchParams(window.location.search).get('reset');
  if(passwordResetToken)return <PasswordResetScreen token={passwordResetToken} onComplete={()=>{window.history.replaceState({},'',window.location.pathname);setAuth((current)=>({setupRequired:false,registrationEnabled:current?.registrationEnabled??false,passwordResetEnabled:current?.passwordResetEnabled??false,authenticated:false,account:null}))}}/>;
  if (invitationToken) return <InvitationScreen token={invitationToken} currentAccount={auth.authenticated?auth.account:null} onSignOut={logout} onAuthenticated={authenticated} />;
  if (!auth.authenticated) return <AuthScreen setupRequired={auth.setupRequired} registrationEnabled={auth.registrationEnabled} passwordResetEnabled={auth.passwordResetEnabled} onAuthenticated={authenticated} />;

  return (
    <SecretsContext.Provider value={secrets}><TreeEditingContext.Provider value={{nodes,busy,cutNodeId,dragNodeId,setCutNodeId,setDragNodeId,addChild:(id)=>{setSelectedId(id);setDialog('child')},focusNode:focusTreeNode,canMove:canMoveBranch,moveNode:(nodeId,parentId)=>perform(async()=>{await moveTreeNode(nodeId,parentId);setCutNodeId(null);setDragNodeId(null)}),removeNode:(node)=>perform(()=>removeTreeNode(node))}}><main className="shell">
      <header className="topbar">
        <a className="brand" href="/">Work Graph</a>
        <nav aria-label="Primary navigation"><button className={view === 'structure' ? 'active' : ''} onClick={openCurrentTree}>Tree</button><button className={view === 'history' ? 'active' : ''} onClick={openHistory}>History</button><button className={view === 'my-work' ? 'active' : ''} onClick={() => setView('my-work')}>My work{myWork.length>0&&<i className="work-count">{myWork.length}</i>}</button>{(currentWorkspace?.workDistributionMode==='exchange'||exchangeOffers.length>0)&&<button className={view === 'exchange' ? 'active' : ''} onClick={() => setView('exchange')}>Exchange{exchangeOffers.length>0&&<i className="work-count">{exchangeOffers.length}</i>}</button>}<button className={view === 'profile' ? 'active' : ''} onClick={() => setView('profile')}>My profile</button>{canAdminister&&<button className={`admin-entry ${isAdminView?'active':''}`} onClick={()=>setView('directory')}>Administration{priorities.conflicts.some((item)=>!item.chosenRootId)&&<i className="nav-alert"/>}</button>}</nav>
        <div className="account-menu"><span>{auth.account?.displayName}</span><button onClick={() => perform(logout)}>Sign out</button></div>
      </header>

      {error && <div className="error" role="alert">{error}<button onClick={() => setError('')} aria-label="Dismiss">×</button></div>}

      <section className="workspace" id="work">
        <aside className="sidebar">
          <label htmlFor="workspace-picker">Workspace</label>
          {workspaces.length ? (
            <select id="workspace-picker" value={workspaceId} onChange={(event) => { setSelectedId(null); setWorkspaceId(event.target.value); }}>
              {workspaces.map((workspace) => <option key={workspace.id} value={workspace.id}>{workspace.name}</option>)}
            </select>
          ) : <p className="muted">No workspace yet.</p>}
          <button className="secondary" type="button" onClick={() => setDialog('workspace')}>+ New workspace</button>
          {isAdminView ? <div className="admin-sidebar"><p className="sidebar-heading">Administration</p><small>Workspace settings, access, automation, and shared knowledge.</small><div className="admin-nav"><button className={view==='directory'?'selected':''} onClick={()=>setView('directory')}>People &amp; knowledge</button><button className={view==='priorities'?'selected':''} onClick={()=>setView('priorities')}>Priority decisions{priorities.conflicts.some((item)=>!item.chosenRootId)&&<i className="nav-alert"/>}</button><button className={view==='diagnostics'?'selected':''} onClick={()=>setView('diagnostics')}>Diagnostics</button><button className={view==='modules'?'selected':''} onClick={()=>setView('modules')}>Modules</button><button className={view==='secrets'?'selected':''} onClick={()=>setView('secrets')}>Secrets</button><button className={view==='api-access'?'selected':''} onClick={()=>setView('api-access')}>API access</button></div><button className="back-to-work" onClick={()=>setView('structure')}>← Back to work tree</button></div> : <><div className="sidebar-heading"><span>Goals</span>{currentWorkspace && <button className="icon-button" title="New root goal" onClick={() => setDialog('root')}>+</button>}</div><div className="root-list">
            {roots.map((root) => <button key={root.id} className={selected?.rootId === root.id ? 'selected' : ''} onClick={() => {returnToCurrent();setSelectedId(root.id);setView('structure')}}><span className={`dot ${root.lifecycleStatus}`} />{root.title}{criticalNodeIds.has(root.id)&&<i className="critical-mark" title="A descendant branch is temporarily critical">!</i>}</button>)}
          </div></>}
        </aside>

        <section className="canvas">
          {!currentWorkspace ? (
            <EmptyState title="Your work forest starts here" text="Create a workspace and an independent root goal such as “Build a house”." action="Create workspace" onAction={() => setDialog('workspace')} />
          ) : view === 'my-work' ? (
            <MyWorkView items={myWork} rootScores={priorities.rootScores} criticalNodeIds={criticalNodeIds} onOpen={(nodeId)=>{setPreviewRevision(null);setSelectedId(nodeId);setView('structure')}} />
          ) : view === 'exchange' ? (
            <ExchangeView offers={exchangeOffers} busy={busy} perform={perform} onSubmit={submitBid} onWithdraw={withdrawBid} onOpen={(nodeId)=>{setPreviewRevision(null);setSelectedId(nodeId);setView('structure')}} />
          ) : view === 'directory' ? (
            <AdminSurface title="People & knowledge"><DirectoryView workspaceId={workspaceId} workspace={currentWorkspace} directory={directory} knowledgeSubjects={knowledgeSubjects} busy={busy} perform={perform} onDistributionChange={updateWorkDistribution} onCreateActor={createActor} onCreateCapability={createCapability} onAssignCapability={assignCapability} onRemoveCapability={removeCapability} onUpdateMemberRole={updateMemberRole} onRemoveMember={removeMember} onCreateKnowledgeSubject={createKnowledgeSubject} onAssignKnowledge={assignKnowledge} onRemoveKnowledge={removeKnowledge} /></AdminSurface>
          ) : view === 'profile' ? (
            <ProfileView profile={profile} capabilities={directory.capabilities} knowledgeSubjects={knowledgeSubjects} busy={busy} perform={perform} onAddSkill={addMySkill} onRemoveSkill={removeMySkill} onAddKnowledge={addMyKnowledge} onRemoveKnowledge={removeMyKnowledge} />
          ) : view === 'secrets' ? (
            <AdminSurface title="Secrets"><SecretsView items={secrets} busy={busy} perform={perform} onCreate={createSecret} onRotate={rotateSecret} onDisable={disableSecret}/></AdminSurface>
          ) : view === 'modules' ? (
            <AdminSurface title="Modules"><ModulesView items={moduleInstallations} busy={busy} perform={perform} onSave={updateModulePolicy}/></AdminSurface>
          ) : view === 'api-access' ? (
            <AdminSurface title="API access"><ServiceAccountsView items={serviceAccounts} credential={issuedCredential} busy={busy} perform={perform} onDismissCredential={()=>setIssuedCredential(null)} onCreate={createServiceAccount} onRotate={rotateServiceAccount} onRevoke={revokeServiceAccount}/></AdminSurface>
          ) : view === 'priorities' ? (
            <AdminSurface title="Priority decisions"><PriorityView inbox={priorities} busy={busy} perform={perform} onDecide={decidePriority} /></AdminSurface>
          ) : view === 'diagnostics' ? (
            <AdminSurface title="Diagnostics"><DiagnosticSettingsView value={diagnosticSettings} diagnostics={diagnostics} busy={busy} perform={perform} onSave={updateDiagnosticSettings}/></AdminSurface>
          ) : !selected ? (
            <EmptyState title="Create your first goal" text="Every task will remain connected to a larger purpose." action="Create root goal" onAction={() => setDialog('root')} />
          ) : (
            <>
              <div className="graph-pane">
                <div className="graph-heading"><div><p className="eyebrow">{view==='history' ? previewRevision===null?'Workspace history':`Historical snapshot · revision ${previewRevision}` : focusedBranch?'Focused branch':'Work structure'}</p><h2>{view==='history'?'Tree through time':focusedBranch?.title??currentWorkspace.name}</h2></div>{view==='history' ? <div className="history-navigation"><div><button className="secondary" disabled={busy||historyPosition<=0} onClick={()=>moveHistory(-1)}>← Earlier</button><span>{historyPosition>=0?`${historyPosition+1} of ${historyPoints.length}`:`${historyPoints.length} revisions`}</span><button className="secondary" disabled={busy||historyPosition<0||historyPosition>=historyPoints.length-1} onClick={()=>moveHistory(1)}>Later →</button></div><button className="secondary return-current" onClick={openCurrentTree}>Return to current tree</button></div> : <div className="graph-tools"><button className="tree-search-trigger" onClick={()=>setTreeSearchOpen(true)} aria-label="Search tree">⌕ <kbd>{navigator.platform.includes('Mac')?'⌘':'Ctrl'} K</kbd></button>{focusedBranch&&<button className="secondary" onClick={()=>setFocusedBranchId(null)}>Show full tree</button>}<div className="graph-legend"><span className="move-hint">↗ Drag a planned task to move its branch</span><span><i className="rank-example">1</i> Your next work</span><span><i className="dot planned" /> Planned</span><span><i className="dot active" /> Active</span><span><i className="dot review" /> Review</span><span><i className="dot closed" /> Closed</span></div></div>}</div>
                {previewRevision===null&&!onboardingDismissed&&!onboardingComplete&&<OnboardingGuide steps={onboardingSteps} onDismiss={dismissOnboarding}/>}
                {treeSearchOpen&&previewRevision===null&&<div className="tree-search"><input autoFocus value={treeSearchQuery} placeholder="Find a task or outcome…" onChange={(event)=>setTreeSearchQuery(event.target.value)} onKeyDown={(event)=>{if(event.key==='Escape'){setTreeSearchOpen(false);setTreeSearchQuery('')}}}/><button aria-label="Close search" onClick={()=>{setTreeSearchOpen(false);setTreeSearchQuery('')}}>×</button>{treeSearchQuery&&<div className="tree-search-results">{searchResults.map((node)=><button key={node.id} onClick={()=>revealTreeNode(node.id)}><strong>{node.title}</strong><small>{node.lifecycleStatus} · {node.parentId?'task':'root goal'}</small></button>)}{searchResults.length===0&&<p>No matching task</p>}</div>}</div>}
                {selectedPath.length>1&&<nav className="tree-breadcrumb" aria-label="Selected task path">{selectedPath.map((node,index)=><span key={node.id}>{index>0&&<i>›</i>}<button onClick={()=>revealTreeNode(node.id)}>{node.title}</button></span>)}</nav>}
                {previewRevision !== null && <div className="snapshot-banner">You are viewing revision {previewRevision}. Historical data is read-only.</div>}
                {previewRevision===null&&cutNodeId&&<div className="move-clipboard"><span><strong>{nodes.find((node)=>node.id===cutNodeId)?.title}</strong> is ready to move. Select its new parent and press <kbd>{navigator.platform.includes('Mac')?'⌘':'Ctrl'}+V</kbd>.</span><button onClick={()=>setCutNodeId(null)}>Cancel</button></div>}
                {previewRevision===null&&removedBranches.length>0&&<details className="removed-branches"><summary>Removed branches <span>{removedBranches.length}</span></summary><div>{removedBranches.map((branch)=><article key={branch.id}><div><strong>{branch.title}</strong><small>{branch.nodeCount} task{branch.nodeCount===1?'':'s'} · removed in revision {branch.removedRevision}</small></div><button className="secondary" disabled={busy} onClick={()=>perform(()=>restoreTreeNode(branch))}>Restore to original parent</button></article>)}</div></details>}
                {previewRevision===null&&diagnostics.length>0&&<DiagnosticsPanel items={diagnostics} onOpenTask={revealTreeNode} onOpenKnowledge={canAdminister?()=>setView('directory'):undefined}/>}
                <div className="graph-scroll"><div className="work-forest">{visibleStructureRoots.map((root) => <GraphBranch key={root.id} node={root} nodes={structureNodes} criticalNodeIds={criticalNodeIds} diagnosticsByNode={previewRevision===null?diagnosticsByNode:new Map()} personalRanks={previewRevision===null?personalRanks:new Map()} selectedId={structureSelected?.id ?? null} collapsedBranches={branchCollapse} readOnly={view==='history'} onToggle={toggleBranch} onSelect={setSelectedId} />)}</div></div>
              </div>
              <div className="detail-grid">
                {previewRevision === null ? <NodeEditor key={`${selected.id}-${selected.updatedRevision}`} node={selected} circle={circle} capabilities={directory.capabilities} knowledgeSubjects={knowledgeSubjects} workflowModules={workflowModules} criticality={criticality} busy={busy} onAddRequirement={(capabilityId) => perform(() => addRequirement(capabilityId))} onAddKnowledgeRequirement={(subjectId) => perform(() => addKnowledgeRequirement(subjectId))} onAddWorkflowStep={(input)=>perform(()=>addWorkflowStep(input))} onUpdateWorkflowStep={(stepId,input)=>perform(()=>updateWorkflowStep(stepId,input))} onMoveWorkflowStep={(stepId,direction)=>perform(()=>moveWorkflowStep(stepId,direction))} onDeleteWorkflowStep={(stepId)=>perform(()=>deleteWorkflowStep(stepId))} onSelectBid={(stepId)=>perform(()=>selectWorkflowStepBid(stepId))} onReturn={(stepId,reason)=>perform(()=>returnForRevision(stepId,reason))} onMarkCritical={(reason,until) => perform(()=>markCritical(reason,until))} onRevokeCritical={(signalId)=>perform(()=>revokeCritical(signalId))} onAction={(action) => perform(() => performWorkAction(action))} onSave={(input) => perform(() => saveNode(input))} /> : <SnapshotDetails node={structureSelected} revision={previewRevision} change={previewChange} activity={previewActivity} nodes={structureNodes} />}
                <ActivityPanel workspaceId={workspaceId} items={activity} activeRevision={previewRevision} busy={busy} historyMode={view==='history'} onAppend={(older)=>setActivity((current)=>[...current,...older.filter((item)=>!current.some((existing)=>existing.id===item.id))])} onPreview={(revision,entityId)=>{setView('history');void previewAtRevision(revision,entityId)}} />
              </div>
            </>
          )}
        </section>
      </section>

      {dialog && <CreateDialog kind={dialog} parent={selected} busy={busy} firstRun={dialog==='workspace'&&firstWorkspacePrompt} onClose={() => {setDialog(null);setFirstWorkspacePrompt(false)}} onSubmit={(name, outcome) => perform(() => dialog === 'workspace' ? createWorkspace(name, outcome) : createNode(name, outcome, dialog === 'child' ? selectedId : null))} />}
    </main></TreeEditingContext.Provider></SecretsContext.Provider>
  );
}

function AuthShell({ children }: { children: ReactNode }) {
  return <main className="auth-shell"><section className="auth-intro"><a className="brand" href="/">Work Graph</a><div><p className="eyebrow">Purpose-driven work</p><h1>Make the structure of work visible.</h1><p>Connect every task to its purpose, its people, and its history.</p></div></section><section className="auth-panel">{children}</section></main>;
}

function AdminSurface({title,children}:{title:string;children:ReactNode}) {
  return <section className="admin-surface"><header><div><p className="eyebrow">Administration</p><strong>{title}</strong></div><span>Changes here affect the entire workspace</span></header>{children}</section>;
}

function DiagnosticSettingsView({value,diagnostics,busy,perform,onSave}:{value:DiagnosticSettings;diagnostics:Diagnostic[];busy:boolean;perform:(action:()=>Promise<void>)=>Promise<void>;onSave:(input:DiagnosticSettings)=>Promise<void>}){
  const [draft,setDraft]=useState(value);useEffect(()=>setDraft(value),[value]);
  const field=(key:keyof DiagnosticSettings,label:string,help:string,min:number,max:number)=><label><span>{label}</span><input type="number" min={min} max={max} value={draft[key]} onChange={(event)=>setDraft({...draft,[key]:Number(event.target.value)})}/><small>{help}</small></label>;
  return <section className="diagnostic-settings-view"><header><div><h2>Attention thresholds</h2><p>Choose when structural and timing signals become useful. These settings never change task status or priority.</p></div><strong>{diagnostics.length} active signal{diagnostics.length===1?'':'s'}</strong></header><div className="diagnostic-setting-grid">{field('wideBranchChildren','Open children in one branch','Suggest grouping after this many direct open child tasks.',4,50)}{field('requesterReviewDays','Days awaiting requester review','Warn only after all descendant work is closed.',1,90)}{field('blockedWorkDays','Days continuously blocked','Uses the latest transition into blocked status.',1,365)}</div><button disabled={busy} onClick={()=>perform(()=>onSave(draft))}>Save thresholds</button></section>;
}

function AuthScreen({ setupRequired, registrationEnabled,passwordResetEnabled, onAuthenticated }: { setupRequired: boolean; registrationEnabled: boolean;passwordResetEnabled:boolean; onAuthenticated: (account: Account,newAccount?:boolean) => Promise<void> }) {
  const [mode, setMode] = useState<'login'|'register'|'forgot'>(setupRequired ? 'register' : 'login');
  const [displayName, setDisplayName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  async function submit(event: FormEvent) {
    event.preventDefault(); setBusy(true); setError('');
    try {
      if(mode==='forgot'){await api('/api/v1/auth/password-reset',{method:'POST',body:JSON.stringify({email})});setMode('login');setError('If an account exists, a reset link has been sent.');return}
      const registering=mode==='register';
      const account = await api<Account>(registering ? '/api/v1/auth/register' : '/api/v1/auth/login', { method: 'POST', body: JSON.stringify(registering ? { displayName, email, password } : { email, password }) });
      await onAuthenticated(account,registering);
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Authentication failed'); }
    finally { setBusy(false); }
  }
  const registering=mode==='register';
  const forgot=mode==='forgot';
  return <AuthShell><div className="auth-card"><p className="eyebrow">{setupRequired ? 'First-time setup' : forgot?'Account recovery':registering ? 'New account' : 'Welcome back'}</p><h2>{setupRequired ? 'Create the system owner' : forgot?'Reset your password':registering ? 'Create your account' : 'Sign in'}</h2><p>{setupRequired ? 'This first account owns the existing workspace and can create new ones.' : forgot?'Enter your account email and we will send a one-time reset link.':registering ? 'Start with your own workspace. Joining another workspace still requires an invitation.' : 'Use your Work Graph account to continue.'}</p>{error && <div className={error.startsWith('If an account')?'auth-notice':'auth-error'} role="status">{error}</div>}<form onSubmit={submit}>{registering && <label>Your name<input autoComplete="name" placeholder="Full name" value={displayName} required onChange={(event) => setDisplayName(event.target.value)} /></label>}<label>Email<input type="email" autoComplete="email" placeholder="name@example.com" value={email} required onChange={(event) => setEmail(event.target.value)} /></label>{!forgot&&<label>Password<input type="password" autoComplete={registering ? 'new-password' : 'current-password'} minLength={10} value={password} required onChange={(event) => setPassword(event.target.value)} /></label>}{registering && <small>Use at least 10 characters. Your password is stored only as a secure hash.</small>}<button disabled={busy}>{busy ? 'Please wait…' : forgot?'Send reset link':setupRequired ? 'Create owner account' : registering ? 'Create account' : 'Sign in'}</button></form>{!setupRequired&&!registering&&!forgot&&passwordResetEnabled&&<button className="auth-switch" type="button" disabled={busy} onClick={()=>{setMode('forgot');setError('')}}>Forgot your password?</button>}{!setupRequired&&registrationEnabled&&<button className="auth-switch" type="button" disabled={busy} onClick={()=>{setMode(registering||forgot?'login':'register');setError('')}}>{registering||forgot?'Back to sign in':'New to Work Graph? Create an account'}</button>}</div></AuthShell>;
}

function PasswordResetScreen({token,onComplete}:{token:string;onComplete:()=>void}){
  const [password,setPassword]=useState('');const [confirmation,setConfirmation]=useState('');const [busy,setBusy]=useState(false);const [error,setError]=useState('');
  async function submit(event:FormEvent){event.preventDefault();if(password!==confirmation){setError('Passwords do not match');return}setBusy(true);setError('');try{await api(`/api/v1/auth/password-reset/${encodeURIComponent(token)}`,{method:'POST',body:JSON.stringify({password})});onComplete()}catch(cause){setError(cause instanceof Error?cause.message:'The password could not be reset')}finally{setBusy(false)}}
  return <AuthShell><div className="auth-card"><p className="eyebrow">Account recovery</p><h2>Choose a new password</h2><p>This one-time link expires after 30 minutes. Resetting your password signs out every existing session.</p>{error&&<div className="auth-error" role="alert">{error}</div>}<form onSubmit={submit}><label>New password<input type="password" autoComplete="new-password" minLength={10} value={password} required onChange={(event)=>setPassword(event.target.value)}/></label><label>Confirm password<input type="password" autoComplete="new-password" minLength={10} value={confirmation} required onChange={(event)=>setConfirmation(event.target.value)}/></label><button disabled={busy}>{busy?'Resetting…':'Reset password'}</button></form></div></AuthShell>
}

function InvitationScreen({ token, currentAccount, onSignOut, onAuthenticated }: { token: string; currentAccount:Account|null; onSignOut:()=>Promise<void>; onAuthenticated: (account: Account) => Promise<void> }) {
  const [invitation, setInvitation] = useState<Invitation | null>(null);
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  useEffect(() => { api<Invitation>(`/api/v1/auth/invitations/${encodeURIComponent(token)}`).then(setInvitation).catch((cause: Error) => setError(cause.message)); }, [token]);
  async function submit(event: FormEvent) {
    event.preventDefault(); setBusy(true); setError('');
    try { const account = await api<Account>(`/api/v1/auth/invitations/${encodeURIComponent(token)}/accept`, { method: 'POST', body: JSON.stringify({ password }) }); window.history.replaceState({}, '', window.location.pathname); await onAuthenticated(account); }
    catch (cause) { setError(cause instanceof Error ? cause.message : 'The invitation could not be accepted'); }
    finally { setBusy(false); }
  }
  const accountMatches=Boolean(invitation&&currentAccount&&invitation.email.toLowerCase()===currentAccount.email.toLowerCase());
  return <AuthShell><div className="auth-card"><p className="eyebrow">Workspace invitation</p><h2>{invitation ? `Join ${invitation.workspaceName}` : 'Loading invitation…'}</h2>{invitation&&currentAccount&&!accountMatches?<><p>This invitation is for <strong>{invitation.email}</strong>, but you are signed in as <strong>{currentAccount.email}</strong>.</p><button disabled={busy} onClick={async()=>{setBusy(true);await onSignOut();setBusy(false)}}>Sign out to continue</button></>:invitation?<><p><strong>{invitation.displayName}</strong>, you will join as {invitation.workspaceRole}.</p><form onSubmit={submit}>{!currentAccount&&<><label>Password<input type="password" autoComplete="current-password" minLength={10} value={password} required onChange={(event) => setPassword(event.target.value)} /></label><small>Use your existing password, or choose at least 10 characters if this is your first account.</small></>}<button disabled={busy}>{busy ? 'Joining…' : currentAccount?'Join workspace':'Continue and join'}</button></form></>:null}{error && <div className="auth-error" role="alert">{error}</div>}</div></AuthShell>;
}

function OnboardingGuide({steps,onDismiss}:{steps:OnboardingStep[];onDismiss:()=>void}){
  const completed=steps.filter((step)=>step.complete).length;
  const next=steps.find((step)=>!step.complete);
  return <details className="onboarding-guide"><summary aria-label={`Getting started: ${completed} of ${steps.length} complete. Next: ${next?.title}`}><span><i>{completed}/{steps.length}</i><span><strong>Getting started</strong><small>{next?.title}</small></span></span><b aria-hidden="true">›</b></summary><div><ol>{steps.map((step)=><li className={step.complete?'complete':''} key={step.id}><i>{step.complete?'✓':'○'}</i><span><strong>{step.title}</strong><small>{step.description}</small></span>{!step.complete&&step.onAction&&<button className="secondary" onClick={step.onAction}>{step.action}</button>}</li>)}</ol><button className="onboarding-dismiss" onClick={onDismiss}>Hide this guide</button></div></details>;
}

function DiagnosticsPanel({items,onOpenTask,onOpenKnowledge}:{items:Diagnostic[];onOpenTask:(nodeId:string)=>void;onOpenKnowledge?:()=>void}){
  return <details className="diagnostics-panel"><summary><span><i>!</i> Attention needed</span><strong>{items.length}</strong></summary><div className="diagnostic-list">{items.map((item)=>{const knowledge=Boolean(item.subjectId);return <article className={item.severity} key={item.id}><span className="diagnostic-symbol">!</span><div><small>{knowledge?`Knowledge · ${item.subjectName}`:item.nodeTitle}</small><h3>{item.title}</h3><p>{item.explanation}</p><ul>{item.evidence.map((evidence)=><li key={evidence}>{evidence}</li>)}</ul></div>{item.nodeId?<button className="secondary" onClick={()=>onOpenTask(item.nodeId)}>Show task</button>:onOpenKnowledge&&<button className="secondary" onClick={onOpenKnowledge}>Review knowledge</button>}</article>})}</div></details>;
}

function GraphBranch({ node, nodes, criticalNodeIds, diagnosticsByNode, personalRanks, selectedId, collapsedBranches, readOnly=false, onToggle, onSelect }: { node: WorkNode; nodes: WorkNode[]; criticalNodeIds: Set<string>; diagnosticsByNode:Map<string,Diagnostic[]>; personalRanks: Map<string,PersonalRank>; selectedId: string | null; collapsedBranches:Record<string,boolean>; readOnly?:boolean; onToggle:(nodeId:string,collapsed:boolean)=>void; onSelect: (id: string) => void }) {
  const children = nodes.filter((candidate) => candidate.parentId === node.id);
  const descendantIds=new Set<string>([node.id]);let found=true;while(found){found=false;for(const candidate of nodes){if(candidate.parentId&&descendantIds.has(candidate.parentId)&&!descendantIds.has(candidate.id)){descendantIds.add(candidate.id);found=true}}}
  const completedBranch=children.length>0&&nodes.filter((candidate)=>descendantIds.has(candidate.id)).every((candidate)=>candidate.lifecycleStatus==='closed');
  const collapsed=children.length>0&&(collapsedBranches[node.id]??completedBranch);
  const personalRank=personalRanks.get(node.id);
  const nodeDiagnostics=diagnosticsByNode.get(node.id)??[];
  const treeEditing=useContext(TreeEditingContext);
  const movable=Boolean(!readOnly&&treeEditing&&node.parentId&&node.lifecycleStatus==='planned'&&!treeEditing.busy);
  const movingId=treeEditing?.dragNodeId??treeEditing?.cutNodeId;
  const validTarget=Boolean(movingId&&treeEditing?.canMove(movingId,node.id));
  return <div className="graph-branch">
    <div className={`graph-node-shell ${selectedId===node.id?'selected':''}`}>{children.length>0&&<button className={`branch-toggle ${collapsed?'collapsed':''}`} aria-label={`${collapsed?'Expand':'Collapse'} ${node.title}`} aria-expanded={!collapsed} onClick={()=>onToggle(node.id,!collapsed)}>{collapsed?'›':'‹'}</button>}<button data-node-id={node.id} draggable={movable} className={`graph-node ${selectedId === node.id ? 'selected' : ''} ${personalRank?'personal-action':''} ${treeEditing?.cutNodeId===node.id?'cut':''} ${treeEditing?.dragNodeId===node.id?'dragging':''} ${validTarget?'move-target':''}`} onClick={() => onSelect(node.id)} onDragStart={(event)=>{if(!movable)return;event.dataTransfer.effectAllowed='move';event.dataTransfer.setData('text/plain',node.id);treeEditing?.setDragNodeId(node.id)}} onDragEnd={()=>treeEditing?.setDragNodeId(null)} onDragOver={(event)=>{if(validTarget){event.preventDefault();event.dataTransfer.dropEffect='move'}}} onDrop={(event)=>{event.preventDefault();if(movingId&&validTarget)void treeEditing?.moveNode(movingId,node.id)}} title={movable?'Drag this branch onto another planned task, or use Ctrl/Cmd+X':''}>
      <span className={`dot ${node.lifecycleStatus}`} /><span className="graph-node-copy"><strong>{node.title}</strong><small>{personalRank?`${personalRank.stepStatus==='active'?'In progress':personalRank.stepStatus==='review'?'Needs your review':'Next'} · ${personalRank.stepName}`:`${node.parentId ? 'Task' : 'Root goal'} · ${node.lifecycleStatus}`}</small></span>{personalRank&&<i className={`personal-rank ${personalRank.stepStatus}`}>{personalRank.rank}</i>}{nodeDiagnostics.length>0&&<i className="diagnostic-mark" title={`${nodeDiagnostics.length} unresolved diagnostic${nodeDiagnostics.length===1?'':'s'}`}>{nodeDiagnostics.length}</i>}{criticalNodeIds.has(node.id)&&<i className="critical-mark">!</i>}
    </button>{treeEditing&&!readOnly&&<details className="graph-node-menu" onClick={(event)=>event.stopPropagation()}><summary aria-label={`Actions for ${node.title}`}>•••</summary><div><button onClick={()=>treeEditing.addChild(node.id)}>Add child task <kbd>N</kbd></button>{children.length>0&&<button onClick={()=>treeEditing.focusNode(node.id)}>Focus branch</button>}{movable&&<><button onClick={(event)=>{treeEditing.setCutNodeId(node.id);(event.currentTarget.closest('details') as HTMLDetailsElement|null)?.removeAttribute('open')}}>Cut branch</button><button className="danger" onClick={()=>{if(window.confirm(`Remove “${node.title}” and every task below it from the current tree?`))void treeEditing.removeNode(node)}}>Remove branch</button></>}</div></details>}</div>
    {!collapsed&&children.length > 0 && <div className="graph-children">{children.map((child) => <GraphBranch key={child.id} node={child} nodes={nodes} criticalNodeIds={criticalNodeIds} diagnosticsByNode={diagnosticsByNode} personalRanks={personalRanks} selectedId={selectedId} collapsedBranches={collapsedBranches} readOnly={readOnly} onToggle={onToggle} onSelect={onSelect} />)}</div>}
  </div>;
}

function MyWorkView({items,rootScores,criticalNodeIds,onOpen}:{items:MyWorkItem[];rootScores:Record<string,number>;criticalNodeIds:Set<string>;onOpen:(nodeId:string)=>void}){
  const ordered=orderPersonalWork(items,rootScores,criticalNodeIds);
  return <section className="my-work-view"><header><div><p className="eyebrow">Personal queue</p><h2>My work</h2></div><p>Active and available stages from every branch, ordered by current context. Open an item in the tree to act with its purpose and surrounding work visible.</p></header>{ordered.length===0?<div className="my-work-empty"><h3>Nothing is waiting for you</h3><p>Work appears here when an active stage matches all of your roles, skills, and entity knowledge.</p></div>:<div className="my-work-list">{ordered.map((item)=>{const critical=Boolean(item.criticalUntil||criticalNodeIds.has(item.nodeId));return <article key={item.stepId} className={`${item.stepStatus} ${critical?'critical':''}`}><div className="my-work-state"><span>{item.stepStatus}</span>{critical&&<i>Critical</i>}</div><div className="my-work-copy"><small>{item.rootTitle}</small><h3>{item.stepName}</h3><p>{item.nodeTitle}</p><em>Requested by {item.requester} · stage {item.stepPosition}</em></div><div className="my-work-actions"><button onClick={()=>onOpen(item.nodeId)}>Show in tree</button></div></article>})}</div>}</section>;
}

function ExchangeView({offers,busy,perform,onSubmit,onWithdraw,onOpen}:{offers:ExchangeOffer[];busy:boolean;perform:(action:()=>Promise<void>)=>Promise<void>;onSubmit:(offer:ExchangeOffer,minutes:number)=>Promise<void>;onWithdraw:(offer:ExchangeOffer)=>Promise<void>;onOpen:(nodeId:string)=>void}){
  return <section className="exchange-view"><header><div><p className="eyebrow">Human-stage exchange</p><h2>Available work</h2></div><p>These ready stages match your skills and entity knowledge. Propose the time you expect your own stage to take.</p></header>{offers.length===0?<div className="my-work-empty"><h3>No matching offers right now</h3><p>New work appears here when a ready human stage matches every required capability and knowledge subject.</p></div>:<div className="exchange-list">{offers.map((offer)=><ExchangeOfferCard key={`${offer.stepId}-${offer.myBid?.updatedAt??'new'}`} offer={offer} busy={busy} perform={perform} onSubmit={onSubmit} onWithdraw={onWithdraw} onOpen={onOpen}/>)}</div>}</section>;
}

function ExchangeOfferCard({offer,busy,perform,onSubmit,onWithdraw,onOpen}:{offer:ExchangeOffer;busy:boolean;perform:(action:()=>Promise<void>)=>Promise<void>;onSubmit:(offer:ExchangeOffer,minutes:number)=>Promise<void>;onWithdraw:(offer:ExchangeOffer)=>Promise<void>;onOpen:(nodeId:string)=>void}){
  const existing=offer.myBid?.promisedDurationMinutes??0;
  const initialUnit=existing>=1440&&existing%1440===0?'days':'hours';
  const [unit,setUnit]=useState<'hours'|'days'>(initialUnit);
  const [amount,setAmount]=useState(existing?String(existing/(initialUnit==='days'?1440:60)):'');
  const minutes=Math.round(Number(amount)*(unit==='days'?1440:60));
  const active=offer.myBid?.status==='active';
  return <article className={active?'bid-active':''}><div className="exchange-context"><small>{offer.rootTitle} · stage {offer.stepPosition}</small><h3>{offer.stepName}</h3><p>{offer.nodeTitle}</p><em>Requested by {offer.requester}</em><div>{offer.requirements.map((item)=><span key={`cap-${item}`}>{item}</span>)}{offer.knowledgeRequirements.map((item)=><span className="knowledge" key={`knowledge-${item}`}>{item}</span>)}</div></div><div className="exchange-bid"><span>{offer.activeBidCount} active bid{offer.activeBidCount===1?'':'s'}</span>{active&&<strong>Your proposed duration: {formatDuration(existing)}</strong>}<div className="duration-input"><input aria-label={`Proposed duration for ${offer.stepName}`} type="number" min="0.25" step="0.25" value={amount} placeholder="Time" onChange={(event)=>setAmount(event.target.value)}/><select aria-label="Duration unit" value={unit} onChange={(event)=>setUnit(event.target.value as 'hours'|'days')}><option value="hours">hours</option><option value="days">days</option></select></div><button disabled={busy||!Number.isFinite(minutes)||minutes<1||minutes>525600} onClick={()=>perform(()=>onSubmit(offer,minutes))}>{active?'Update estimate':'Submit estimate'}</button>{active&&<button className="secondary" disabled={busy} onClick={()=>perform(()=>onWithdraw(offer))}>Withdraw</button>}<button className="text-button" onClick={()=>onOpen(offer.nodeId)}>Show in tree</button></div></article>;
}

function formatDuration(minutes:number){if(minutes===0)return '0 minutes';if(minutes%1440===0)return `${minutes/1440} day${minutes===1440?'':'s'}`;if(minutes%60===0)return `${minutes/60} hour${minutes===60?'':'s'}`;return `${minutes} minutes`}

function PriorityView({ inbox, busy, perform, onDecide }: { inbox: PriorityInbox; busy: boolean; perform: (action: () => Promise<void>) => Promise<void>; onDecide: (conflict: PriorityConflict, chosenRootId: string | null, basis: 'goal' | 'requester' | 'unknown') => Promise<void> }) {
  const unresolved = inbox.conflicts.filter((item) => !item.chosenRootId);
  const resolved = inbox.conflicts.filter((item) => item.chosenRootId);
  return <section className="priority-view"><header><div><p className="eyebrow">Decision inbox</p><h2>Resolve priority from context</h2></div><p>Questions appear when you are eligible for work under different goals. Decisions remain explicit and explain why one branch comes first.</p></header>{inbox.conflicts.length===0?<div className="priority-empty"><h3>No priority conflict right now</h3><p>Conflicts will appear when the same person matches actionable tasks from two or more root goals.</p></div>:<><div className="priority-list">{unresolved.map((conflict)=><PriorityQuestion key={`${conflict.left.rootId}-${conflict.right.rootId}`} conflict={conflict} busy={busy} perform={perform} onDecide={onDecide} />)}</div>{resolved.length>0&&<section className="resolved-decisions"><h3>Current decisions</h3>{resolved.map((conflict)=>{const chosen=conflict.chosenRootId===conflict.left.rootId?conflict.left:conflict.right; return <div key={`${conflict.left.rootId}-${conflict.right.rootId}`}><strong>{chosen.rootTitle}</strong><span>comes before {chosen.rootId===conflict.left.rootId?conflict.right.rootTitle:conflict.left.rootTitle}</span><small>Based on {conflict.decisionBasis} · {conflict.decidedAt&&new Date(conflict.decidedAt).toLocaleString()}</small><button className="secondary" onClick={()=>onDecide(conflict,null,'unknown')}>Reconsider</button></div>})}</section>}</>}</section>;
}

function PriorityQuestion({ conflict, busy, perform, onDecide }: { conflict: PriorityConflict; busy: boolean; perform: (action: () => Promise<void>) => Promise<void>; onDecide: (conflict: PriorityConflict, chosenRootId: string | null, basis: 'goal' | 'requester' | 'unknown') => Promise<void> }) {
  const sameRequester=conflict.left.requester.id===conflict.right.requester.id;
  const askRequester=conflict.decisionBasis==='unknown'&&!sameRequester;
  return <article className="priority-question"><p className="eyebrow">Priority question · affects {conflict.affectedActors.map((actor)=>actor.displayName).join(', ')}</p><h3>{askRequester?'Whose judgment do you trust more for this decision?':'Which goal is more important right now?'}</h3><div className="priority-options"><button disabled={busy} onClick={()=>perform(()=>onDecide(conflict,conflict.left.rootId,askRequester?'requester':'goal'))}><strong>{askRequester?conflict.left.requester.displayName:conflict.left.rootTitle}</strong><span>{conflict.left.taskTitle}</span><small>{conflict.left.rootTitle}</small></button><span>or</span><button disabled={busy} onClick={()=>perform(()=>onDecide(conflict,conflict.right.rootId,askRequester?'requester':'goal'))}><strong>{askRequester?conflict.right.requester.displayName:conflict.right.rootTitle}</strong><span>{conflict.right.taskTitle}</span><small>{conflict.right.rootTitle}</small></button></div><div className="priority-fallback"><button className="secondary" disabled={busy} onClick={()=>perform(()=>onDecide(conflict,null,'unknown'))}>I don't have enough context</button>{!askRequester&&!sameRequester&&<span>Next, the system will ask which requester you trust more for this decision.</span>}{sameRequester&&<span>Both tasks have the same requester; the decision remains unresolved.</span>}{askRequester&&<span>The decision will remain unresolved until more context is available.</span>}</div></article>;
}

function EmptyState({ title, text, action, onAction }: { title: string; text: string; action: string; onAction: () => void }) {
  return <article className="empty-state"><div className="empty-mark">⌘</div><h2>{title}</h2><p>{text}</p><button onClick={onAction}>{action}</button></article>;
}

function NodeEditor({ node, circle, capabilities, knowledgeSubjects, workflowModules, criticality, busy, onAddRequirement, onAddKnowledgeRequirement, onAddWorkflowStep, onUpdateWorkflowStep, onMoveWorkflowStep, onDeleteWorkflowStep,onSelectBid, onReturn, onMarkCritical, onRevokeCritical, onAction, onSave }: { node: WorkNode; circle: WorkCircle | null; capabilities: Capability[]; knowledgeSubjects: KnowledgeSubject[]; workflowModules: WorkflowModule[]; criticality: CriticalitySignal[]; busy: boolean; onAddRequirement: (capabilityId: string) => void; onAddKnowledgeRequirement: (subjectId: string) => void; onAddWorkflowStep: (input: NewWorkflowStep) => void; onUpdateWorkflowStep: (stepId:string,input:WorkflowStepUpdate) => void; onMoveWorkflowStep: (stepId:string,direction:'earlier'|'later') => void; onDeleteWorkflowStep: (stepId:string) => void;onSelectBid:(stepId:string)=>void; onReturn: (stepId:string,reason:string)=>void; onMarkCritical: (reason: string, until: string) => void; onRevokeCritical: (signalId: string) => void; onAction: (action: WorkAction) => void; onSave: (input: Pick<WorkNode, 'title' | 'desiredOutcome' | 'lifecycleStatus'>) => void }) {
  const [title, setTitle] = useState(node.title);
  const [outcome, setOutcome] = useState(node.desiredOutcome);
  const [editingDetails,setEditingDetails]=useState(false);
  const status = node.lifecycleStatus;
  return <article className="editor">
    <div className="editor-top"><div><p className="eyebrow">{node.parentId ? 'Task' : 'Root goal'} · revision {node.updatedRevision}</p><h2>{node.title}</h2></div><button className={`task-edit-trigger ${editingDetails?'active':''}`} aria-label={`Edit ${node.parentId?'task':'goal'}`} onClick={()=>setEditingDetails(true)}><span aria-hidden="true">✎</span>Edit</button></div>
    {!editingDetails&&<div className="task-summary"><span className={`task-status ${status}`}><i className={`dot ${status}`}/>{status}</span><p>{node.desiredOutcome||'No desired outcome has been described yet.'}</p></div>}
    {editingDetails&&<form className="task-edit-form" onSubmit={(event) => { event.preventDefault(); onSave({ title, desiredOutcome: outcome, lifecycleStatus: status }); }}>
      <label>Title<input value={title} maxLength={500} required onChange={(event) => setTitle(event.target.value)} /></label>
      <label>Desired outcome<textarea value={outcome} rows={6} placeholder="What will be true when this work is complete?" onChange={(event) => setOutcome(event.target.value)} /></label>
      <div className="form-actions"><span>Changes are preserved in the workspace history.</span><div><button type="button" className="secondary" disabled={busy} onClick={()=>{setTitle(node.title);setOutcome(node.desiredOutcome);setEditingDetails(false)}}>Cancel</button><button disabled={busy}>{busy ? 'Saving…' : 'Save changes'}</button></div></div>
    </form>}
    <CriticalityPanel node={node} signals={criticality.filter((signal)=>signal.workNodeId===node.id)} busy={busy} onMark={onMarkCritical} onRevoke={onRevokeCritical} />
    <WorkCirclePanel lifecycleStatus={node.lifecycleStatus} canEdit={node.lifecycleStatus==='planned'} circle={circle} capabilities={capabilities} knowledgeSubjects={knowledgeSubjects} workflowModules={workflowModules} busy={busy} onAddRequirement={onAddRequirement} onAddKnowledgeRequirement={onAddKnowledgeRequirement} onAddWorkflowStep={onAddWorkflowStep} onUpdateWorkflowStep={onUpdateWorkflowStep} onMoveWorkflowStep={onMoveWorkflowStep} onDeleteWorkflowStep={onDeleteWorkflowStep} onSelectBid={onSelectBid} onReturn={onReturn} onAction={onAction} />
  </article>;
}

function CriticalityPanel({ node, signals, busy, onMark, onRevoke }: { node: WorkNode; signals: CriticalitySignal[]; busy: boolean; onMark: (reason: string, until: string) => void; onRevoke: (id: string) => void }) {
  const [reason,setReason]=useState(''); const [until,setUntil]=useState('');
  const [expanded,setExpanded]=useState(signals.length>0);useEffect(()=>{if(signals.length>0)setExpanded(true)},[signals.length]);
  return <details className="criticality-panel" open={expanded} onToggle={(event)=>setExpanded(event.currentTarget.open)}><summary><span>Branch criticality</span><small>{signals.length?`${signals.length} active signal${signals.length===1?'':'s'}`:'Temporary exception'}</small></summary><div className="criticality-content"><p>A critical signal propagates from this {node.parentId?'task':'goal'} to every ancestor until its deadline.</p>{signals.map((signal)=><article key={signal.id}><div><strong>Critical until {new Date(signal.criticalUntil).toLocaleString()}</strong><span>{signal.reason}</span></div><button className="secondary" disabled={busy} onClick={()=>onRevoke(signal.id)}>Revoke</button></article>)}<form onSubmit={(event)=>{event.preventDefault(); onMark(reason,until); setReason(''); setUntil('');}}><input aria-label="Criticality reason" placeholder="Why must this branch come first?" value={reason} required onChange={(event)=>setReason(event.target.value)} /><input aria-label="Critical until" type="datetime-local" value={until} required onChange={(event)=>setUntil(event.target.value)} /><button disabled={busy}>Mark critical</button></form></div></details>;
}

function WorkCirclePanel({ lifecycleStatus, canEdit, circle, capabilities, knowledgeSubjects, workflowModules, busy, onAddRequirement, onAddKnowledgeRequirement, onAddWorkflowStep, onUpdateWorkflowStep, onMoveWorkflowStep, onDeleteWorkflowStep,onSelectBid, onReturn, onAction }: { lifecycleStatus: WorkNode['lifecycleStatus']; canEdit: boolean; circle: WorkCircle | null; capabilities: Capability[]; knowledgeSubjects: KnowledgeSubject[]; workflowModules: WorkflowModule[]; busy: boolean; onAddRequirement: (capabilityId: string) => void; onAddKnowledgeRequirement: (subjectId: string) => void; onAddWorkflowStep: (input: NewWorkflowStep) => void; onUpdateWorkflowStep: (stepId:string,input:WorkflowStepUpdate) => void; onMoveWorkflowStep: (stepId:string,direction:'earlier'|'later') => void; onDeleteWorkflowStep: (stepId:string) => void;onSelectBid:(stepId:string)=>void; onReturn:(stepId:string,reason:string)=>void; onAction: (action: WorkAction) => void }) {
  const [selected, setSelected] = useState('');
  const [selectedSubject, setSelectedSubject] = useState('');
  const [stepIntent,setStepIntent]=useState(''); const [stepType,setStepType]=useState<StepType|null>(null); const [stepName,setStepName]=useState(''); const [stepCapability,setStepCapability]=useState(''); const [stepSubject,setStepSubject]=useState('');
  const [selectedModuleId,setSelectedModuleId]=useState(''); const [moduleConfiguration,setModuleConfiguration]=useState<Record<string,string|number>>({});
  const [returnStep,setReturnStep]=useState(''); const [returnReason,setReturnReason]=useState('');
  const [addingStep,setAddingStep]=useState(false);
  const [editingStepId,setEditingStepId]=useState('');
  if (!circle) return <section className="work-circle"><p className="muted">Loading task circle…</p></section>;
  const available = capabilities.filter((capability) => !circle.requirements.some((requirement) => requirement.id === capability.id));
  const availableSubjects = knowledgeSubjects.filter((subject) => !circle.knowledgeRequirements.some((requirement) => requirement.id === subject.id));
  const normalizedIntent=stepIntent.trim().toLocaleLowerCase();
  const intentTerms=normalizedIntent.split(/[^\p{L}\p{N}+#.]+/u).filter((term)=>term.length>1);
  const humanMatches=[...capabilities.map((item)=>({id:item.id,label:item.name,kind:item.capabilityType as string,source:'capability'})),...knowledgeSubjects.map((item)=>({id:item.id,label:item.name,kind:item.subjectType as string,source:'knowledge'}))].filter((item)=>{const candidate=`${item.label} ${item.kind}`.toLocaleLowerCase();return normalizedIntent&&(normalizedIntent.includes(item.label.toLocaleLowerCase())||intentTerms.some((term)=>candidate.includes(term)))}).slice(0,5);
  const moduleMatches=workflowModules.filter((module)=>normalizedIntent&&[module.name,...module.searchTerms].some((term)=>normalizedIntent.includes(term.toLocaleLowerCase())||intentTerms.some((word)=>term.toLocaleLowerCase().includes(word))));
  const selectedModule=workflowModules.find((module)=>module.moduleId===selectedModuleId)??null;
  const resetComposer=()=>{setStepIntent('');setStepType(null);setStepName('');setStepCapability('');setStepSubject('');setSelectedModuleId('');setModuleConfiguration({});setAddingStep(false)};
  const chooseModule=(module:WorkflowModule)=>{setStepType(module.stepType);setStepName(module.name);setSelectedModuleId(module.moduleId);setModuleConfiguration(Object.fromEntries(module.configurationSchema.fields.filter((field)=>field.default!==undefined).map((field)=>[field.key,field.default!]))) };
  const missingRequiredModuleField=selectedModule?.configurationSchema.fields.some((field)=>field.required&&(moduleConfiguration[field.key]===undefined||String(moduleConfiguration[field.key]).trim()===''))??false;
  const addStep=(event:FormEvent)=>{event.preventDefault();if(!stepType)return;const base={name:stepName.trim(),stepType};if(stepType==='human')onAddWorkflowStep({...base,capabilityId:stepCapability||undefined,subjectId:stepSubject||undefined});else if(selectedModule)onAddWorkflowStep({...base,moduleId:selectedModule.moduleId,moduleVersion:selectedModule.moduleVersion,configuration:moduleConfiguration});resetComposer()};
  const currentStep=circle.workflowSteps.find((step)=>['active','assigned','failed','ready'].includes(step.stepStatus))??null;
  const requesterHasBall=lifecycleStatus==='review'||lifecycleStatus==='closed'||!currentStep;
  const responsibilityLabel=requesterHasBall
    ? lifecycleStatus==='review'?`With ${circle.requester.displayName} for acceptance`:lifecycleStatus==='closed'?`Accepted by ${circle.requester.displayName}`:`With ${circle.requester.displayName}`
    : currentStep!.stepType==='human'?(currentStep!.claimedBy?`With ${currentStep!.claimedBy.displayName}`:'Available to a matching performer'):`With ${currentStep!.name}`;
  const circleItemCount=1+circle.workflowSteps.length+(canEdit?1:0);
  const circlePoint=(index:number)=>{if(circleItemCount===1)return {x:50,y:50};const angle=(-Math.PI/2)+(index*2*Math.PI/circleItemCount);return {x:50+40*Math.cos(angle),y:50+39*Math.sin(angle)}};
  const circlePoints=Array.from({length:circleItemCount},(_,index)=>circlePoint(index));
  const circleLine=(index:number)=>{const from=circlePoints[index];const to=circlePoints[(index+1)%circlePoints.length];const dx=to.x-from.x;const dy=to.y-from.y;const length=Math.hypot(dx,dy)||1;const inset=Math.min(9,length*.28);return {x1:from.x+dx/length*inset,y1:from.y+dy/length*inset,x2:to.x-dx/length*inset,y2:to.y-dy/length*inset}};
  return <section className="work-circle"><div className="circle-heading"><div><p className="eyebrow">Task route</p><h3>Task circle</h3><p>Work moves from the requester through each stage, then returns to the requester for acceptance.</p></div><span>{circle.workflowSteps.length} stage{circle.workflowSteps.length===1?'':'s'}</span></div>
    <div className="circle-loop" aria-label="Task execution circle">
      <div className="circle-owner-state"><span className="responsibility-ball" aria-hidden="true" />{responsibilityLabel}</div>
      <div className={`circle-map items-${Math.min(circleItemCount,9)}`} aria-label="Task execution order">
        {circleItemCount>1&&<svg className="circle-connectors" viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true"><defs><marker id="circle-arrow" markerWidth="4" markerHeight="4" refX="3.2" refY="2" orient="auto"><path d="M0,0 L4,2 L0,4 Z" /></marker></defs>{circlePoints.map((_,index)=>{const line=circleLine(index);return <line key={index} {...line} markerEnd="url(#circle-arrow)" />})}</svg>}
        <div className={`circle-node requester ${requesterHasBall?'has-ball':''}`} style={{left:`${circlePoints[0].x}%`,top:`${circlePoints[0].y}%`}}><span className="circle-node-number">●</span><small>Requester</small><strong>{circle.requester.displayName}</strong><em>{lifecycleStatus==='review'?'Accept or return':'Starts · accepts · closes'}</em>{requesterHasBall&&<span className="card-ball" title="Current responsibility" />}</div>
        {circle.workflowSteps.map((step,index)=><button type="button" key={step.id} className={`circle-node step ${step.stepStatus} ${step.stepType} ${editingStepId===step.id?'selected':''} ${currentStep?.id===step.id&&!requesterHasBall?'has-ball':''}`} style={{left:`${circlePoints[index+1].x}%`,top:`${circlePoints[index+1].y}%`}} onClick={()=>{resetComposer();setEditingStepId(step.id)}}><span className="circle-node-number">{step.position}</span><small>{step.stepType==='api'?'HTTP':step.stepType==='script'?'Bash':step.stepType}</small><strong>{step.name}</strong><em>{step.stepStatus==='active'?'Happening now':step.stepStatus==='assigned'?'Performer selected':step.stepStatus==='ready'?'Ready to start':step.stepStatus==='completed'?'Completed':step.stepStatus==='failed'?'Needs attention':'Waiting'}</em>{currentStep?.id===step.id&&!requesterHasBall&&<span className="card-ball" title="Current responsibility" />}</button>)}
        {canEdit&&<button type="button" className={`circle-node add ${addingStep?'selected':''}`} style={{left:`${circlePoints[circleItemCount-1].x}%`,top:`${circlePoints[circleItemCount-1].y}%`}} onClick={()=>{setEditingStepId('');setAddingStep(true)}}><span className="circle-node-number">+</span><strong>Add stage</strong><em>Person or automation</em></button>}
        <div className="circle-map-center"><strong>Task circle</strong><span>Work travels clockwise</span><small>Every route returns to its requester</small></div>
      </div>
      {addingStep&&<form className="add-step-popover step-composer circle-composer" onSubmit={addStep}><div className="composer-heading"><strong>Add next stage</strong><small>{stepType?'Configure the selected stage':'Describe who or what should act'}</small></div>{!stepType&&<><input autoFocus aria-label="Next stage intent" placeholder="Try: Python, Product A, bash, API request…" value={stepIntent} onChange={(event)=>setStepIntent(event.target.value)} /><div className="composer-results">{moduleMatches.map((module)=><button type="button" key={module.moduleId} onClick={()=>chooseModule(module)}><b>{module.stepType==='api'?'↗':module.stepType==='script'?'⌘':'◇'}</b><span><strong>{module.name}</strong><small>{module.description}</small></span></button>)}{humanMatches.map((item)=><button type="button" key={`${item.source}-${item.id}`} onClick={()=>{setStepType('human');setStepName(stepIntent.trim()||item.label);if(item.source==='capability')setStepCapability(item.id);else setStepSubject(item.id)}}><b>●</b><span><strong>{item.label}</strong><small>{item.kind} · human work</small></span></button>)}{normalizedIntent&&!humanMatches.length&&!moduleMatches.length&&<p>No exact match. Create the role, skill, or entity in Directory first.</p>}</div></>}{stepType&&<><div className={`selected-step-kind ${stepType}`}><b>{stepType==='human'?'●':stepType==='api'?'↗':'⌘'}</b><span><strong>{stepType==='human'?'Human stage':selectedModule?.name??'Automation stage'}</strong><small>{stepType==='human'?'Matched from your directory':((selectedModule?.publisher??'Unknown publisher')+' · '+(selectedModule?.moduleVersion??''))}</small></span><button type="button" onClick={()=>setStepType(null)}>Change</button></div><input aria-label="Stage name" placeholder="Stage name" value={stepName} required onChange={(event)=>setStepName(event.target.value)} />{stepType==='human'&&<><select aria-label="Stage role or skill" value={stepCapability} onChange={(event)=>setStepCapability(event.target.value)}><option value="">Role or skill (optional)…</option>{capabilities.map((item)=><option key={item.id} value={item.id}>{item.name} · {item.capabilityType}</option>)}</select><select aria-label="Stage entity knowledge" value={stepSubject} onChange={(event)=>setStepSubject(event.target.value)}><option value="">Entity knowledge (optional)…</option>{knowledgeSubjects.map((item)=><option key={item.id} value={item.id}>{item.name}</option>)}</select></>}{selectedModule&&<ModuleConfigurationEditor module={selectedModule} value={moduleConfiguration} onChange={setModuleConfiguration} />}<p className="runner-notice">Saved as a configured stage. Execution stays disabled until the isolated runner is installed.</p></>}<div className="composer-actions"><button type="button" className="secondary" onClick={resetComposer}>Cancel</button>{stepType&&<button disabled={busy||!stepName.trim()||missingRequiredModuleField}>Add to circle</button>}</div></form>}
    </div>
    {editingStepId&&<WorkflowStepEditor editable={canEdit} canSelect={circle.requester.id===circle.currentActor.id} key={editingStepId} step={circle.workflowSteps.find((step)=>step.id===editingStepId)!} stepCount={circle.workflowSteps.length} capabilities={capabilities} knowledgeSubjects={knowledgeSubjects} modules={workflowModules} busy={busy} onSave={(input)=>onUpdateWorkflowStep(editingStepId,input)} onMove={(direction)=>onMoveWorkflowStep(editingStepId,direction)} onDelete={()=>{onDeleteWorkflowStep(editingStepId);setEditingStepId('')}} onSelectBid={()=>{onSelectBid(editingStepId);setEditingStepId('')}} />}
    <div className="workflow-actions">{circle.permissions.canClaim && <button disabled={busy} onClick={() => onAction('claim')}>Start current stage</button>}{circle.permissions.canSubmit && <button disabled={busy} onClick={() => onAction('submit')}>Complete current stage</button>}{circle.permissions.canClose && <button disabled={busy} onClick={() => onAction('close')}>Accept and close</button>}{circle.permissions.canReopen && <button className="secondary" disabled={busy} onClick={() => onAction('reopen')}>Reopen task</button>}{circle.permissions.canCancel && <button className="danger" disabled={busy} onClick={() => onAction('cancel')}>Cancel HTTP execution</button>}{circle.permissions.canRetry && <button disabled={busy} onClick={() => onAction('retry')}>Retry failed HTTP stage</button>}</div>
    <details className="matching-details"><summary><span>Current stage matching</span><small>{circle.matchingPerformers.length} matching performer{circle.matchingPerformers.length===1?'':'s'} · edit requirements</small></summary><div className="requirements-block"><h4>Required roles and skills</h4><div className="capability-chips">{circle.requirements.map((requirement) => <span key={requirement.id} className={requirement.capabilityType}>{requirement.name}</span>)}{circle.requirements.length === 0 && <small>No role or skill requirements yet</small>}</div>{available.length > 0 && <div className="assign-capability"><select aria-label="Task requirement" value={selected} onChange={(event) => setSelected(event.target.value)}><option value="">Add role or skill…</option>{available.map((capability) => <option key={capability.id} value={capability.id}>{capability.name} · {capability.capabilityType}</option>)}</select><button disabled={busy || !selected} onClick={() => { onAddRequirement(selected); setSelected(''); }}>Add</button></div>}<h4 className="knowledge-requirement-title">Required entity knowledge</h4><div className="knowledge-requirement-chips">{circle.knowledgeRequirements.map((requirement) => <span key={requirement.id}>{requirement.name}<small>{requirement.subjectType}</small></span>)}{circle.knowledgeRequirements.length === 0 && <small>No entity knowledge required yet</small>}</div>{availableSubjects.length > 0 && <div className="assign-capability"><select aria-label="Task knowledge requirement" value={selectedSubject} onChange={(event) => setSelectedSubject(event.target.value)}><option value="">Add entity knowledge…</option>{availableSubjects.map((subject) => <option key={subject.id} value={subject.id}>{subject.name} · {subject.subjectType}</option>)}</select><button disabled={busy || !selectedSubject} onClick={() => { onAddKnowledgeRequirement(selectedSubject); setSelectedSubject(''); }}>Add</button></div>}<div className="matches-block"><h4>Matching performers · strongest evidence first</h4>{circle.requirements.length === 0 && circle.knowledgeRequirements.length === 0 ? <p>Add requirements to calculate candidates.</p> : circle.matchingPerformers.length ? <div>{circle.matchingPerformers.map((actor,index) => <span key={actor.id} className={actor.matchQuality}><b>{index+1}</b>{actor.displayName}<small>{actor.matchQuality==='verified'?'Admin verified':actor.matchQuality==='partially_verified'?`${actor.confirmedRequirements}/${actor.requirementCount} verified`:'Self-reported'}</small></span>)}</div> : <p>No actor currently satisfies every requirement.</p>}</div></div></details>
    {circle.openDescendants.length>0&&<div className="child-work-block"><h4>Open child work</h4><p>The result can be submitted and reviewed now, but it cannot be closed until every task in its branches is closed.</p><div>{circle.openDescendants.map((child)=><span key={child.id} style={{marginLeft:`${Math.min(child.depth-1,4)*.65}rem`}}><i className={`dot ${child.lifecycleStatus}`} /><strong>{child.title}</strong><small>{child.lifecycleStatus}</small></span>)}</div></div>}
    {circle.permissions.canReturn&&<form className="return-work" onSubmit={(event)=>{event.preventDefault();onReturn(returnStep,returnReason);setReturnStep('');setReturnReason('');}}><div><h4>Return for revision</h4><p>Choose the stage that must be repeated. Every later stage will wait again.</p></div><select aria-label="Stage to repeat" value={returnStep} required onChange={(event)=>setReturnStep(event.target.value)}><option value="">Choose completed stage…</option>{circle.workflowSteps.filter((step)=>step.stepStatus==='completed').map((step)=><option key={step.id} value={step.id}>{step.position}. {step.name}</option>)}</select><textarea aria-label="Revision reason" placeholder="Explain what needs to change" value={returnReason} required rows={3} onChange={(event)=>setReturnReason(event.target.value)} /><button className="secondary" disabled={busy}>Return selected stage</button></form>}
  </section>;
}

function WorkflowStepEditor({editable,canSelect,step,stepCount,capabilities,knowledgeSubjects,modules,busy,onSave,onMove,onDelete,onSelectBid}:{editable:boolean;canSelect:boolean;step:WorkflowStep;stepCount:number;capabilities:Capability[];knowledgeSubjects:KnowledgeSubject[];modules:WorkflowModule[];busy:boolean;onSave:(input:WorkflowStepUpdate)=>void;onMove:(direction:'earlier'|'later')=>void;onDelete:()=>void;onSelectBid:()=>void}) {
  const [name,setName]=useState(step.name); const [capabilityId,setCapabilityId]=useState(step.requirements[0]?.id??''); const [subjectId,setSubjectId]=useState(step.knowledgeRequirements[0]?.id??''); const [distributionMode,setDistributionMode]=useState<DistributionMode>(step.distributionMode??'inherit'); const [configuration,setConfiguration]=useState<Record<string,string|number>>(step.configuration as Record<string,string|number>);
  const module=modules.find((item)=>item.moduleId===step.moduleId&&item.moduleVersion===step.moduleVersion)??null;
  const missing=module?.configurationSchema.fields.some((field)=>field.required&&(configuration[field.key]===undefined||String(configuration[field.key]).trim()===''))??false;
  const activeBids=step.bids.filter((bid)=>bid.status==='active');
  const riskRecommendation=activeBids.length>1&&activeBids.every((bid)=>bid.riskAssessment)?[...activeBids].sort(compareBidRisk)[0]:null;
  return <form className="step-editor" onSubmit={(event)=>{event.preventDefault();onSave({name,capabilityId:capabilityId||undefined,subjectId:subjectId||undefined,distributionMode,configuration})}}><div className="step-editor-heading"><div><p className="eyebrow">Edit route stage {step.position}</p><h4>{step.stepType==='human'?'Human stage':module?.name??'Automation stage'}</h4></div>{editable&&<div className="step-editor-heading-actions"><button type="button" className="danger" disabled={busy} onClick={onDelete}>Delete stage</button></div>}</div>{editable&&<><input aria-label="Edit stage name" value={name} required onChange={(event)=>setName(event.target.value)} />{step.stepType==='human'?<><div className="step-editor-requirements"><select aria-label="Edit stage role or skill" value={capabilityId} onChange={(event)=>setCapabilityId(event.target.value)}><option value="">No role or skill requirement</option>{capabilities.map((item)=><option key={item.id} value={item.id}>{item.name} · {item.capabilityType}</option>)}</select><select aria-label="Edit stage entity knowledge" value={subjectId} onChange={(event)=>setSubjectId(event.target.value)}><option value="">No entity knowledge requirement</option>{knowledgeSubjects.map((item)=><option key={item.id} value={item.id}>{item.name}</option>)}</select></div><label className="step-distribution">How should this stage be offered?<select value={distributionMode} onChange={(event)=>setDistributionMode(event.target.value as DistributionMode)}><option value="inherit">Use workspace setting</option><option value="simple">Simple · direct start</option><option value="exchange">Exchange · duration estimates</option></select></label></>:module?<ModuleConfigurationEditor module={module} value={configuration} onChange={setConfiguration}/>:<p className="runner-notice">The installed module version is unavailable.</p>}</>}{step.effectiveDistributionMode==='exchange'&&step.bids.length>0&&<section className="step-bids"><div><strong>{step.stepStatus==='assigned'?'Selected performer':'Duration estimates'}</strong><span>{riskRecommendation?`Advisory: ${riskRecommendation.actor.displayName} has the shortest risk-adjusted outlook. Selection still uses the shortest proposed estimate.`:'Shortest estimate wins. A risk outlook appears only when every active candidate has at least five completed observations.'}</span></div>{step.bids.map((bid)=><p key={bid.id} className={`${bid.status} ${riskRecommendation?.id===bid.id?'recommended':''}`}><b>{bid.actor.displayName}{riskRecommendation?.id===bid.id&&<i>Advisory</i>}</b><span>{formatDuration(bid.promisedDurationMinutes)}</span><span className="bid-reliability">{formatBidReliability(bid.estimation)}{bid.riskAssessment&&<> · risk outlook <strong>{formatDuration(bid.riskAssessment.adjustedDurationMinutes)}</strong> ({formatDuration(bid.riskAssessment.expectedDurationMinutes)} expected + {formatDuration(bid.riskAssessment.uncertaintyBufferMinutes)} uncertainty)</>}</span><small>{bid.status}</small></p>)}{canSelect&&step.stepStatus==='ready'&&activeBids.length>0&&<button type="button" disabled={busy} onClick={onSelectBid}>Select shortest estimate</button>}</section>}{editable&&<div className="step-editor-actions"><button type="button" className="secondary" disabled={busy||step.position===1} onClick={()=>onMove('earlier')}>← Earlier</button><button type="button" className="secondary" disabled={busy||step.position===stepCount} onClick={()=>onMove('later')}>Later →</button><button disabled={busy||!name.trim()||missing}>Save changes</button></div>}<ExecutionHistory executions={step.executions}/></form>;
}

function compareBidRisk(left:StepBid,right:StepBid){return (left.riskAssessment?.adjustedDurationMinutes??Infinity)-(right.riskAssessment?.adjustedDurationMinutes??Infinity)||left.estimation.longOverrunRate-right.estimation.longOverrunRate||left.promisedDurationMinutes-right.promisedDurationMinutes||new Date(left.submittedAt).getTime()-new Date(right.submittedAt).getTime()||left.actor.id.localeCompare(right.actor.id)}

function formatBidReliability(stats:EstimationStats){if(stats.observationCount===0)return 'Reliability unknown · no completed estimates';const bias=Math.round(stats.meanRelativeError*100);const direction=Math.abs(bias)<1?'on estimate on average':bias>0?`${bias}% slower on average`:`${Math.abs(bias)}% faster on average`;const stability=stats.observationCount<2?'stability unknown':stats.stability<=.15?'stable':stats.stability<=.4?'variable':'unstable';const overruns=stats.longOverrunCount?`${stats.longOverrunCount} long overrun${stats.longOverrunCount===1?'':'s'}`:'no long overruns';return `${stats.observationCount} completed · ${direction} · ${stability} · ${overruns}`}

function ExecutionHistory({executions}:{executions:WorkflowExecution[]}) {
  return <section className="execution-history"><div><h4>Execution history</h4><small>{executions.length} attempt{executions.length===1?'':'s'}</small></div>{executions.length===0?<p>No execution attempts yet.</p>:<ol>{executions.map((execution)=><li key={execution.id}><span className={`execution-state ${execution.executionStatus}`}>{execution.executionStatus}</span><div><strong>Attempt {execution.attemptNumber}</strong><small>{execution.startedBy?.displayName??'Automated runner'} · {new Date(execution.startedAt).toLocaleString()}</small>{execution.finishedAt&&<small>Finished {new Date(execution.finishedAt).toLocaleString()}</small>}{execution.promisedDurationMinutes!==null&&<div className="estimate-observation"><span>Agreed <b>{formatDuration(execution.promisedDurationMinutes)}</b></span>{execution.actualDurationMinutes!==null&&<span>Actual <b>{formatMeasuredDuration(execution.actualDurationMinutes)}</b></span>}{execution.relativeError!==null&&<i className={execution.relativeError>0?'late':'early'}>{formatRelativeError(execution.relativeError)}</i>}</div>}{execution.returnReason&&<em>{execution.returnReason}</em>}{execution.errorMessage&&<em>{execution.errorMessage}</em>}{Object.keys(execution.result??{}).length>0&&<details><summary>Result</summary><pre>{JSON.stringify(execution.result,null,2)}</pre></details>}</div></li>)}</ol>}</section>;
}

function formatMeasuredDuration(minutes:number){if(minutes<1)return '< 1 minute';return formatDuration(Math.round(minutes))}
function formatRelativeError(value:number){const amount=Math.round(Math.abs(value)*100);if(amount<1)return 'on estimate';return value>0?`${amount}% over`:`${amount}% under`}

function ModuleConfigurationEditor({ module, value, onChange }: { module: WorkflowModule; value: Record<string,string|number>; onChange: (value: Record<string,string|number>) => void }) {
  const secrets=useContext(SecretsContext);
  const update=(key:string,next:string|number)=>onChange({...value,[key]:next});
  return <div className="module-fields">{module.configurationSchema.fields.map((field)=>{
    const current=value[field.key]??'';
    if(field.type==='secret')return <label key={field.key}>{field.label}<select value={current} onChange={(event)=>update(field.key,event.target.value)}><option value="">No credential</option>{secrets.filter((item)=>item.enabled).map((item)=><option key={item.id} value={item.id}>{item.name} · v{item.version}</option>)}</select><small>The value is injected only inside the runner.</small></label>;
    if((field.key==='secretPlacement'||field.key==='secretHeader')&&!value.secretId)return null;
    if(field.key==='secretHeader'&&value.secretPlacement!=='header')return null;
    if(field.type==='select')return <label key={field.key}>{field.label}<select required={field.required} value={current} onChange={(event)=>update(field.key,event.target.value)}>{field.options?.map((option)=><option key={option} value={option}>{option}</option>)}</select></label>;
    if(field.type==='textarea'||field.type==='code')return <label key={field.key}>{field.label}<textarea className={field.type==='code'?'code-input':undefined} required={field.required} placeholder={field.placeholder} rows={field.type==='code'?7:4} value={current} onChange={(event)=>update(field.key,event.target.value)} /></label>;
    return <label key={field.key}>{field.label}<input type={field.type==='number'?'number':field.type==='url'?'url':'text'} required={field.required} placeholder={field.placeholder} min={field.min} max={field.max} value={current} onChange={(event)=>update(field.key,field.type==='number'?Number(event.target.value):event.target.value)} /></label>;
  })}</div>;
}

function SnapshotDetails({ node, revision, change, activity, nodes }: { node: WorkNode | null; revision: number; change:ChangeEvent|null; activity:ActivityItem|null; nodes:WorkNode[] }) {
  if (!node) return <article className="snapshot-details"><p className="muted">This revision contains no visible nodes.</p></article>;
  const parentName=(parentId:string|null)=>parentId?nodes.find((item)=>item.id===parentId)?.title??'Another branch':'Root goal';
  const changedFields=change?.beforeState?[{label:'Title',before:change.beforeState.title,after:change.afterState.title},{label:'Desired outcome',before:change.beforeState.desiredOutcome||'Not specified',after:change.afterState.desiredOutcome||'Not specified'},{label:'Status',before:change.beforeState.lifecycleStatus,after:change.afterState.lifecycleStatus},{label:'Parent',before:parentName(change.beforeState.parentId),after:parentName(change.afterState.parentId)}].filter((item)=>item.before!==item.after):[];
  return <article className="snapshot-details"><p className="eyebrow">Read-only node · revision {revision}</p><h2>{node.title}</h2><dl><div><dt>Status</dt><dd><span className={`dot ${node.lifecycleStatus}`} />{node.lifecycleStatus}</dd></div><div><dt>Desired outcome</dt><dd>{node.desiredOutcome || 'Not specified at this revision'}</dd></div><div><dt>Last changed</dt><dd>Revision {node.updatedRevision}</dd></div></dl>{change?<section className="revision-change"><header><div><p className="eyebrow">What changed in revision {revision}</p><h3>{change.eventType==='node.created'?'Task created':'Task updated'}</h3></div>{activity&&<span>{activity.actorName} · {new Date(activity.occurredAt).toLocaleString()}</span>}</header>{change.beforeState===null?<p>This task appeared in the tree under <strong>{parentName(change.afterState.parentId)}</strong>.</p>:changedFields.length?<div>{changedFields.map((field)=><article key={field.label}><strong>{field.label}</strong><span><del>{field.before}</del><i>→</i><ins>{field.after}</ins></span></article>)}</div>:<p>The node was included in this revision, but its visible task fields did not change.</p>}</section>:activity&&<section className="revision-change circle-change"><header><div><p className="eyebrow">Task circle · revision {revision}</p><h3>{activity.summary}</h3></div><span>{activity.actorName} · {new Date(activity.occurredAt).toLocaleString()}</span></header><p>{activity.detail||`This event changed the task route for ${activity.nodeTitle||node.title}.`}</p></section>}</article>;
}

function ActivityPanel({ workspaceId,items,activeRevision,busy,historyMode,onAppend,onPreview }: { workspaceId:string;items:ActivityItem[];activeRevision:number|null;busy:boolean;historyMode:boolean;onAppend:(items:ActivityItem[])=>void;onPreview:(revision:number,entityId:string)=>void }) {
  const [filter,setFilter]=useState<'all'|'tree'|'circle'|'workspace'>('all');
  const [query,setQuery]=useState('');
  const [remoteItems,setRemoteItems]=useState<ActivityItem[]|null>(null);
  const [loading,setLoading]=useState(false);
  const [hasMore,setHasMore]=useState(items.length===200);
  const category=(item:ActivityItem):'tree'|'circle'|'workspace'=>item.eventType.startsWith('node.')?'tree':item.eventType.startsWith('workflow')||item.eventType.startsWith('requirement.')||item.eventType.startsWith('knowledge_requirement.')?'circle':'workspace';
  const normalizedQuery=query.trim().toLocaleLowerCase();
  const sourceItems=remoteItems??items;
  const visibleItems=sourceItems.filter((item)=>(filter==='all'||category(item)===filter)&&(!normalizedQuery||`${item.summary} ${item.detail} ${item.nodeTitle} ${item.actorName} ${item.eventType}`.toLocaleLowerCase().includes(normalizedQuery)));
  const count=(value:'tree'|'circle'|'workspace')=>sourceItems.filter((item)=>category(item)===value).length;
  const dayKey=(value:string)=>new Date(value).toLocaleDateString();
  const dayLabel=(value:string)=>{const date=new Date(value);const today=new Date();const yesterday=new Date(today);yesterday.setDate(today.getDate()-1);if(date.toDateString()===today.toDateString())return 'Today';if(date.toDateString()===yesterday.toDateString())return 'Yesterday';return date.toLocaleDateString(undefined,{weekday:'long',year:'numeric',month:'long',day:'numeric'})};
  useEffect(()=>{if(!normalizedQuery){setRemoteItems(null);setHasMore(items.length===200);return}const controller=new AbortController();const timer=window.setTimeout(()=>{setLoading(true);api<ActivityItem[]>(`/api/v1/workspaces/${workspaceId}/activity?limit=200&q=${encodeURIComponent(normalizedQuery)}`,{signal:controller.signal}).then((result)=>{setRemoteItems(result);setHasMore(result.length===200)}).catch((cause)=>{if(cause instanceof Error&&cause.name!=='AbortError')console.error(cause)}).finally(()=>setLoading(false))},250);return()=>{window.clearTimeout(timer);controller.abort()}},[normalizedQuery,workspaceId]);
  async function loadOlder(){const current=remoteItems??items;const oldest=current[current.length-1];if(!oldest)return;setLoading(true);try{const suffix=normalizedQuery?`&q=${encodeURIComponent(normalizedQuery)}`:'';const older=await api<ActivityItem[]>(`/api/v1/workspaces/${workspaceId}/activity?limit=200&before=${encodeURIComponent(oldest.occurredAt)}${suffix}`);setHasMore(older.length===200);if(remoteItems)setRemoteItems([...remoteItems,...older.filter((item)=>!remoteItems.some((existing)=>existing.id===item.id))]);else onAppend(older)}finally{setLoading(false)}}
  return <details className={`history-panel ${historyMode?'history-mode':''}`} open={historyMode||activeRevision!==null}>
    <summary><span>{historyMode?'Revision timeline':'Activity & history'}</span><small>{sourceItems.length} events loaded · {historyMode?'select a moment':'browse revisions'}</small></summary>
    <div className="history-content">{historyMode&&sourceItems.length>0&&<div className="history-controls"><nav className="history-filters" aria-label="History event filters">{([['all','All',sourceItems.length],['tree','Tree',count('tree')],['circle','Task circle',count('circle')],['workspace','Workspace',count('workspace')]] as const).map(([value,label,total])=><button key={value} className={filter===value?'active':''} onClick={()=>setFilter(value)}>{label}<span>{total}</span></button>)}</nav><label className="history-search"><span>⌕</span><input value={query} aria-label="Search history" placeholder="Search all history…" onChange={(event)=>setQuery(event.target.value)}/>{query&&<button aria-label="Clear history search" onClick={()=>setQuery('')}>×</button>}</label></div>}{sourceItems.length === 0&&!loading ? <p className="muted">Activity will appear here.</p> : visibleItems.length===0?<p className="history-empty">{loading?'Searching full history…':'No matching events in the full history.'}</p>:<><ol className="timeline">
      {visibleItems.map((item,index) => {
        const startsDay=index===0||dayKey(visibleItems[index-1].occurredAt)!==dayKey(item.occurredAt);
        return <Fragment key={item.id}>{startsDay&&<li className="timeline-day"><time dateTime={item.occurredAt.slice(0,10)}>{dayLabel(item.occurredAt)}</time></li>}<li>
          <button className={activeRevision === item.workspaceRevision ? 'active' : ''} disabled={busy || !item.canPreview} onClick={() => item.canPreview && item.workspaceRevision && item.nodeId && onPreview(item.workspaceRevision, item.nodeId)}>
            <span className="revision">{item.workspaceRevision ? `r${item.workspaceRevision}` : '•'}</span>
            <span className="event-copy"><strong>{item.summary}</strong><small>{item.nodeTitle&&item.nodeTitle!==item.detail?`${item.nodeTitle}${item.detail?` · ${item.detail}`:''}`:item.detail}</small><time>{item.actorName} · {new Date(item.occurredAt).toLocaleString()}</time></span>
          </button>
        </li></Fragment>;
      })}
    </ol>{historyMode&&hasMore&&<button className="history-load-more" disabled={loading} onClick={()=>void loadOlder()}>{loading?'Loading…':'Load older events'}</button>}</>}</div>
  </details>;
}

function ProfileView({ profile, capabilities, knowledgeSubjects, busy, perform, onAddSkill, onRemoveSkill, onAddKnowledge, onRemoveKnowledge }: { profile: MyProfile | null; capabilities: Capability[]; knowledgeSubjects:KnowledgeSubject[]; busy: boolean; perform: (action: () => Promise<void>) => Promise<void>; onAddSkill: (id: string, level: ClaimEvidence['level'], note: string) => Promise<void>; onRemoveSkill: (id: string) => Promise<void>; onAddKnowledge:(id:string,level:ClaimEvidence['level'],note:string)=>Promise<void>;onRemoveKnowledge:(id:string)=>Promise<void> }) {
  const [selected, setSelected] = useState('');
  const [skillLevel,setSkillLevel]=useState<ClaimEvidence['level']>('working');
  const [skillNote,setSkillNote]=useState('');
  const [selectedKnowledge,setSelectedKnowledge]=useState('');
  const [knowledgeLevel,setKnowledgeLevel]=useState<ClaimEvidence['level']>('working');
  const [knowledgeNote,setKnowledgeNote]=useState('');
  if (!profile) return <section className="profile-view"><p className="muted">Loading your profile…</p></section>;
  const skills = profile.claims.filter((claim) => claim.capabilityType === 'skill');
  const roles = profile.claims.filter((claim) => claim.capabilityType === 'role');
  const skillSelected=(id:string)=>{setSelected(id);const evidence=skills.find((claim)=>claim.id===id)?.evidence.find((item)=>item.source==='self');setSkillLevel(evidence?.level??'working');setSkillNote(evidence?.note??'')};
  const knowledgeSelected=(id:string)=>{setSelectedKnowledge(id);const evidence=profile.knowledge.find((claim)=>claim.id===id)?.evidence.find((item)=>item.source==='self');setKnowledgeLevel(evidence?.level??'working');setKnowledgeNote(evidence?.note??'')};
  return <section className="profile-view"><header><div className="profile-avatar">{profile.displayName.slice(0, 1).toUpperCase()}</div><div><p className="eyebrow">Personal workspace profile</p><h2>{profile.displayName}</h2><span>{profile.workspaceRole} · joined {new Date(profile.createdAt).toLocaleDateString()}</span></div></header><EstimationSummary stats={profile.estimation}/><div className="profile-grid"><section><div className="section-title"><h3>My skills</h3><span>{skills.length}</span></div><p className="profile-help">Describe your experience yourself. Administrative verification remains independent.</p><div className="self-skill-add"><select aria-label="Skill to add or update" value={selected} onChange={(event)=>skillSelected(event.target.value)}><option value="">Choose a skill…</option>{capabilities.filter((item)=>item.capabilityType==='skill').map((skill)=><option key={skill.id} value={skill.id}>{skill.name}</option>)}</select><select aria-label="Skill level" value={skillLevel} onChange={(event)=>setSkillLevel(event.target.value as ClaimEvidence['level'])}><ClaimLevelOptions/></select><textarea aria-label="Skill evidence" value={skillNote} placeholder="How have you used this skill?" onChange={(event)=>setSkillNote(event.target.value)}/><button disabled={busy||!selected} onClick={()=>perform(()=>onAddSkill(selected,skillLevel,skillNote.trim())).then(()=>{setSelected('');setSkillNote('')})}>Add or update</button></div><div className="profile-claims">{skills.map((claim)=><CapabilityClaimCard key={claim.id} claim={claim} busy={busy} onRemove={()=>perform(()=>onRemoveSkill(claim.id))}/>)}{skills.length===0&&<p className="muted">No skills in your profile yet.</p>}</div></section><section><div className="section-title"><h3>Roles</h3><span>{roles.length}</span></div><p className="profile-help">Roles describe responsibilities and can only be assigned by an administrator.</p><div className="profile-claims">{roles.map((claim)=><CapabilityClaimCard key={claim.id} claim={claim} busy={busy}/>)}{roles.length===0&&<p className="muted">No roles assigned yet.</p>}</div></section><section><div className="section-title"><h3>My entity knowledge</h3><span>{profile.knowledge.length}</span></div><p className="profile-help">Record services, projects, or systems you know and the depth of that knowledge.</p><div className="self-skill-add"><select aria-label="Knowledge to add or update" value={selectedKnowledge} onChange={(event)=>knowledgeSelected(event.target.value)}><option value="">Choose an entity…</option>{knowledgeSubjects.map((subject)=><option key={subject.id} value={subject.id}>{subject.name} · {subject.subjectType}</option>)}</select><select aria-label="Knowledge level" value={knowledgeLevel} onChange={(event)=>setKnowledgeLevel(event.target.value as ClaimEvidence['level'])}><ClaimLevelOptions/></select><textarea aria-label="Knowledge evidence" value={knowledgeNote} placeholder="What do you know about this entity?" onChange={(event)=>setKnowledgeNote(event.target.value)}/><button disabled={busy||!selectedKnowledge} onClick={()=>perform(()=>onAddKnowledge(selectedKnowledge,knowledgeLevel,knowledgeNote.trim())).then(()=>{setSelectedKnowledge('');setKnowledgeNote('')})}>Add or update</button></div><div className="profile-claims">{profile.knowledge.map((claim)=><KnowledgeClaimCard key={claim.id} claim={claim} busy={busy} onRemove={()=>perform(()=>onRemoveKnowledge(claim.id))}/>)}{profile.knowledge.length===0&&<p className="muted">No entity knowledge in your profile yet.</p>}</div></section></div></section>;
}

function EstimationSummary({stats}:{stats:EstimationStats}){if(stats.observationCount===0)return <section className="estimation-summary empty"><div><strong>Estimate reliability</strong><span>No completed agreed estimates yet</span></div><p>Coefficients start at zero, but reliability remains unknown until execution history exists.</p></section>;const percent=(value:number)=>`${Math.round(Math.abs(value)*100)}%`;const bias=stats.meanRelativeError>.03?`${percent(stats.meanRelativeError)} slower than agreed on average`:stats.meanRelativeError<-.03?`${percent(stats.meanRelativeError)} faster than agreed on average`:'Usually close to the agreed duration';const stability=stats.observationCount<2?'More observations needed':stats.stability<=.15?'Stable estimates':stats.stability<=.4?'Variable estimates':'Unstable estimates';return <section className="estimation-summary"><div><strong>Estimate reliability</strong><span>{stats.observationCount} completed estimate{stats.observationCount===1?'':'s'}</span></div><ul><li><b>{bias}</b><small>Typical absolute error {percent(stats.meanAbsoluteRelativeError)}</small></li><li><b>{stability}</b><small>Variation {percent(stats.stability)}</small></li><li className={stats.longOverrunCount?'risk':''}><b>{stats.longOverrunCount?`${stats.longOverrunCount} long overrun${stats.longOverrunCount===1?'':'s'}`:'No long overruns observed'}</b><small>{Math.round(stats.longOverrunRate*100)}% of observed stages exceeded 2× the agreed duration</small></li></ul></section>}

function ClaimLevelOptions(){return <><option value="awareness">Awareness</option><option value="working">Working</option><option value="advanced">Advanced</option><option value="expert">Expert</option></>}

function KnowledgeClaimCard({claim,busy,onRemove}:{claim:KnowledgeClaim;busy:boolean;onRemove:()=>void}){const self=claim.sources.includes('self');return <article className="profile-claim"><div><strong>{claim.name}</strong><span><i>{claim.subjectType}</i>{claim.evidence.map((item)=><i key={item.source} className={item.source==='admin'?'verified':''}>{item.source} · {item.level}{item.note?` · ${item.note}`:''}</i>)}</span></div>{self&&<button className="secondary" disabled={busy} onClick={onRemove}>Remove my claim</button>}</article>}

function CapabilityClaimCard({ claim, busy, onRemove }: { claim: CapabilityClaim; busy: boolean; onRemove?: () => void }) {
  const selfDeclared = claim.sources.includes('self');
  return <article className="profile-claim"><div><strong>{claim.name}</strong><span>{claim.evidence.map((item)=><i key={item.source} className={item.source==='admin'?'verified':''}>{item.source} · {item.level}{item.note?` · ${item.note}`:''}</i>)}</span></div>{selfDeclared && onRemove && <button className="secondary" disabled={busy} onClick={onRemove}>Remove my claim</button>}</article>;
}

function ModulesView({items,busy,perform,onSave}:{items:ModuleInstallation[];busy:boolean;perform:(action:()=>Promise<void>)=>Promise<void>;onSave:(item:ModuleInstallation,policy:Pick<ModuleInstallation,'enabled'|'allowedHosts'|'allowSecrets'|'publisherTrusted'>)=>Promise<void>}){
  const [drafts,setDrafts]=useState<Record<string,{enabled:boolean;hosts:string;allowSecrets:boolean;publisherTrusted:boolean}>>({});
  const draftFor=(item:ModuleInstallation)=>drafts[`${item.moduleId}@${item.moduleVersion}`]??{enabled:item.enabled,hosts:item.allowedHosts.join(', '),allowSecrets:item.allowSecrets,publisherTrusted:item.publisherTrusted};
  const update=(item:ModuleInstallation,next:Partial<{enabled:boolean;hosts:string;allowSecrets:boolean;publisherTrusted:boolean}>)=>setDrafts((values)=>({...values,[`${item.moduleId}@${item.moduleVersion}`]:{...draftFor(item),...next}}));
  return <section className="modules-view"><header><div><p className="eyebrow">Execution policy</p><h2>Workflow modules</h2></div><p>Each workspace explicitly controls which automation is available, where it may connect, and whether it may receive credentials.</p></header><div className="module-policy-list">{items.map((item)=>{const draft=draftFor(item);return <article key={`${item.moduleId}@${item.moduleVersion}`}><div className="module-policy-heading"><span className={`module-kind ${item.stepType}`}>{item.stepType}</span><div><strong>{item.name}</strong><small>{item.publisher} · {item.moduleVersion}</small></div><label className="policy-toggle"><input type="checkbox" checked={draft.enabled} disabled={!draft.publisherTrusted} onChange={(event)=>update(item,{enabled:event.target.checked})}/> Enabled</label></div><p>{item.description}</p><div className="publisher-trust"><label className="policy-toggle"><input type="checkbox" checked={draft.publisherTrusted} onChange={(event)=>update(item,{publisherTrusted:event.target.checked,enabled:event.target.checked?draft.enabled:false})}/> Trust publisher: {item.publisher}</label><small>Trust applies to this exact module version in this workspace.</small></div>{item.stepType==='api'&&<div className="module-policy-fields"><label>Exact allowed hosts<input value={draft.hosts} placeholder="api.example.com, hooks.example.com" onChange={(event)=>update(item,{hosts:event.target.value})}/><small>No schemes, paths, ports, or wildcards.</small></label><label className="policy-toggle"><input type="checkbox" checked={draft.allowSecrets} onChange={(event)=>update(item,{allowSecrets:event.target.checked})}/> Allow this module to use workspace secrets</label></div>}<button disabled={busy||draft.enabled&&!draft.publisherTrusted} onClick={()=>perform(()=>onSave(item,{enabled:draft.enabled,allowedHosts:draft.hosts.split(',').map((host)=>host.trim()).filter(Boolean),allowSecrets:draft.allowSecrets,publisherTrusted:draft.publisherTrusted}))}>Save policy</button></article>})}</div></section>;
}

function ServiceAccountsView({items,credential,busy,perform,onDismissCredential,onCreate,onRotate,onRevoke}:{items:ServiceAccount[];credential:ServiceCredential|null;busy:boolean;perform:(action:()=>Promise<void>)=>Promise<void>;onDismissCredential:()=>void;onCreate:(name:string,scopes:ServiceScope[])=>Promise<void>;onRotate:(id:string)=>Promise<void>;onRevoke:(id:string)=>Promise<void>}){
  const [name,setName]=useState('');
  const [scopes,setScopes]=useState<ServiceScope[]>(['work:read']);
  const toggle=(scope:ServiceScope)=>setScopes((current)=>current.includes(scope)?current.filter((item)=>item!==scope):[...current,scope]);
  const submit=(event:FormEvent)=>{event.preventDefault();if(!name.trim()||scopes.length===0)return;perform(()=>onCreate(name.trim(),scopes)).then(()=>setName(''))};
  return <section className="service-accounts-view"><header><div><p className="eyebrow">Integration identity</p><h2>API access</h2></div><p>Create a separate identity for an integration gateway. Its changes appear in the tree history without sharing a person’s password.</p></header>
    {credential&&<div className="service-token-reveal" role="status"><div><strong>Copy this token now</strong><span>It will not be shown again. Store it in the integration service, never in source code.</span></div><code>{credential.token}</code><div><button type="button" onClick={()=>navigator.clipboard.writeText(credential.token)}>Copy token</button><button type="button" className="secondary" onClick={onDismissCredential}>I have saved it</button></div></div>}
    <form className="service-account-create" onSubmit={submit}><label>Name<input value={name} required placeholder="e.g. Jira gateway" onChange={(event)=>setName(event.target.value)}/></label><fieldset><legend>Permissions</legend><label><input type="checkbox" checked={scopes.includes('work:read')} onChange={()=>toggle('work:read')}/> Read the tree and history</label><label><input type="checkbox" checked={scopes.includes('work:write')} onChange={()=>toggle('work:write')}/> Create and change branches</label></fieldset><button disabled={busy||!name.trim()||scopes.length===0}>Create token</button></form>
    <div className="service-account-list">{items.map((item)=><article key={item.id} className={item.revokedAt?'disabled':''}><div className="service-account-icon">↔</div><div><strong>{item.name}</strong><span>{item.scopes.map((scope)=>scope==='work:read'?'Read':'Write').join(' · ')}</span><small>{item.revokedAt?`Revoked ${new Date(item.revokedAt).toLocaleString()}`:item.lastUsedAt?`Last used ${new Date(item.lastUsedAt).toLocaleString()}`:'Never used'} · token {item.tokenPrefix}…</small></div><div className="service-account-actions"><button className="secondary" disabled={busy} onClick={()=>{if(window.confirm(`Replace the token for ${item.name}? The current token will stop working immediately.`))perform(()=>onRotate(item.id))}}>Replace token</button>{!item.revokedAt&&<button className="danger" disabled={busy} onClick={()=>{if(window.confirm(`Revoke API access for ${item.name}?`))perform(()=>onRevoke(item.id))}}>Revoke</button>}</div></article>)}{items.length===0&&<p className="muted">No integrations have API access yet.</p>}</div>
  </section>;
}

function SecretsView({items,busy,perform,onCreate,onRotate,onDisable}:{items:WorkspaceSecret[];busy:boolean;perform:(action:()=>Promise<void>)=>Promise<void>;onCreate:(name:string,type:WorkspaceSecret['secretType'],value:string)=>Promise<void>;onRotate:(id:string,value:string)=>Promise<void>;onDisable:(id:string)=>Promise<void>}){
  const [name,setName]=useState('');const [type,setType]=useState<WorkspaceSecret['secretType']>('api_token');const [value,setValue]=useState('');const [replacement,setReplacement]=useState<Record<string,string>>({});
  const submit=(event:FormEvent)=>{event.preventDefault();perform(()=>onCreate(name.trim(),type,value)).then(()=>{setName('');setValue('')})};
  return <section className="secrets-view"><header><div><p className="eyebrow">Workspace security</p><h2>Secrets</h2></div><p>Store credentials once and refer to them by name. Saved values are encrypted and are never shown again.</p></header><form className="secret-create" onSubmit={submit}><label>Name<input value={name} required placeholder="e.g. Slack production token" onChange={(event)=>setName(event.target.value)}/></label><label>Type<select value={type} onChange={(event)=>setType(event.target.value as WorkspaceSecret['secretType'])}><option value="api_token">API token</option><option value="password">Password</option><option value="signing_key">Signing key</option><option value="other">Other</option></select></label><label>Secret value<input type="password" autoComplete="new-password" value={value} required onChange={(event)=>setValue(event.target.value)}/></label><button disabled={busy||!name.trim()||!value}>Store encrypted secret</button></form><div className="secret-list">{items.map((item)=><article key={item.id} className={!item.enabled?'disabled':''}><div className="secret-icon">•••</div><div><strong>{item.name}</strong><span>{item.secretType.replace('_',' ')} · version {item.version}</span><small>{item.enabled?'Enabled':'Disabled'} · updated {new Date(item.updatedAt).toLocaleString()}</small></div><div className="secret-actions"><input type="password" autoComplete="new-password" aria-label={`Replacement value for ${item.name}`} placeholder="New value" value={replacement[item.id]??''} onChange={(event)=>setReplacement((values)=>({...values,[item.id]:event.target.value}))}/><button className="secondary" disabled={busy||!replacement[item.id]} onClick={()=>perform(()=>onRotate(item.id,replacement[item.id])).then(()=>setReplacement((values)=>({...values,[item.id]:''})))}>Replace</button>{item.enabled&&<button className="danger" disabled={busy} onClick={()=>perform(()=>onDisable(item.id))}>Disable</button>}</div></article>)}{items.length===0&&<p className="muted">No secrets have been created. Their values will never appear in this list.</p>}</div></section>;
}

function DirectoryView({ workspaceId, workspace, directory, knowledgeSubjects, busy, perform, onDistributionChange, onCreateActor, onCreateCapability, onAssignCapability,onRemoveCapability,onUpdateMemberRole,onRemoveMember, onCreateKnowledgeSubject, onAssignKnowledge,onRemoveKnowledge }: {
  workspaceId: string;
  workspace:Workspace;
  directory: Directory;
  knowledgeSubjects: KnowledgeSubject[];
  busy: boolean;
  perform: (action: () => Promise<void>) => Promise<void>;
  onDistributionChange:(mode:Workspace['workDistributionMode'])=>Promise<void>;
  onCreateActor: (name: string, type: Actor['actorType']) => Promise<void>;
  onCreateCapability: (name: string, type: Capability['capabilityType']) => Promise<void>;
  onAssignCapability: (actorId: string, capabilityId: string) => Promise<void>;
  onRemoveCapability:(actorId:string,capabilityId:string)=>Promise<void>;
  onUpdateMemberRole:(actorId:string,role:'admin'|'member')=>Promise<void>;
  onRemoveMember:(actorId:string)=>Promise<void>;
  onCreateKnowledgeSubject: (name: string, type: KnowledgeSubject['subjectType']) => Promise<void>;
  onAssignKnowledge: (actorId: string, subjectId: string) => Promise<void>;
  onRemoveKnowledge:(actorId:string,subjectId:string)=>Promise<void>;
}) {
  const [personName, setPersonName] = useState('');
  const [capabilityName, setCapabilityName] = useState('');
  const [capabilityType, setCapabilityType] = useState<Capability['capabilityType']>('role');
  const canAdminister = directory.currentWorkspaceRole === 'owner' || directory.currentWorkspaceRole === 'admin';
  return <section className="directory-view">
    <div className="directory-heading"><div><p className="eyebrow">Workspace directory</p><h2>People and capabilities</h2></div><p>People are matched to work through roles and skills, never selected by name in task requirements.</p></div>
    {canAdminister&&<section className="distribution-policy"><div><strong>Work distribution</strong><span>{workspace.workDistributionMode==='simple'?'People can start matching work directly. Best for personal projects and small teams.':'Matching people propose a duration before human stages are assigned.'}</span></div><select aria-label="Workspace work distribution" value={workspace.workDistributionMode} disabled={busy} onChange={(event)=>perform(()=>onDistributionChange(event.target.value as Workspace['workDistributionMode']))}><option value="simple">Simple · direct start</option><option value="exchange">Exchange · duration estimates</option></select></section>}
    <div className="directory-grid">
      <section><div className="section-title"><h3>People &amp; actors</h3><span>{directory.actors.length}</span></div>
        {canAdminister && <form className="inline-create" onSubmit={(event) => { event.preventDefault(); if (!personName.trim()) return; perform(() => onCreateActor(personName.trim(), 'person')).then(() => setPersonName('')); }}><input aria-label="Person name" placeholder="Add a person without an account" value={personName} onChange={(event) => setPersonName(event.target.value)} /><button disabled={busy}>Add</button></form>}
        <div className="actor-list">{directory.actors.map((actor) => <ActorCard key={actor.id} actor={actor} capabilities={directory.capabilities} busy={busy} currentWorkspaceRole={directory.currentWorkspaceRole} canAdminister={canAdminister} onAssign={(capabilityId) => perform(() => onAssignCapability(actor.id, capabilityId))} onRemove={(capabilityId)=>perform(()=>onRemoveCapability(actor.id,capabilityId))} onUpdateRole={(role)=>perform(()=>onUpdateMemberRole(actor.id,role))} onRemoveMember={()=>perform(()=>onRemoveMember(actor.id))} />)}</div>
      </section>
      <section><div className="section-title"><h3>Capability taxonomy</h3><span>{directory.capabilities.length}</span></div>
        {canAdminister && <form className="capability-create" onSubmit={(event) => { event.preventDefault(); if (!capabilityName.trim()) return; perform(() => onCreateCapability(capabilityName.trim(), capabilityType)).then(() => setCapabilityName('')); }}><input aria-label="Capability name" placeholder="e.g. Engineer or Python" value={capabilityName} onChange={(event) => setCapabilityName(event.target.value)} /><select aria-label="Capability type" value={capabilityType} onChange={(event) => setCapabilityType(event.target.value as Capability['capabilityType'])}><option value="role">Role</option><option value="skill">Skill</option></select><button disabled={busy}>Add</button></form>}
        <div className="capability-list">{directory.capabilities.map((capability) => <div key={capability.id}><span className={`capability-kind ${capability.capabilityType}`}>{capability.capabilityType}</span><strong>{capability.name}</strong></div>)}{directory.capabilities.length === 0 && <p className="muted">Add the first role or skill.</p>}</div>
      </section>
    </div>
    <KnowledgeSubjects subjects={knowledgeSubjects} actors={directory.actors} canAdminister={canAdminister} busy={busy} perform={perform} onCreate={onCreateKnowledgeSubject} onAssign={onAssignKnowledge} onRemove={onRemoveKnowledge} />
    {canAdminister&&<AdministrativeClaims actors={directory.actors} subjects={knowledgeSubjects} busy={busy} perform={perform} onRemoveCapability={onRemoveCapability} onRemoveKnowledge={onRemoveKnowledge}/>}
    {canAdminister && <InvitationManager workspaceId={workspaceId} busy={busy} />}
  </section>;
}

function AdministrativeClaims({actors,subjects,busy,perform,onRemoveCapability,onRemoveKnowledge}:{actors:Actor[];subjects:KnowledgeSubject[];busy:boolean;perform:(action:()=>Promise<void>)=>Promise<void>;onRemoveCapability:(actorId:string,capabilityId:string)=>Promise<void>;onRemoveKnowledge:(actorId:string,subjectId:string)=>Promise<void>}){
  const capabilities=actors.flatMap((actor)=>actor.capabilities.filter((item)=>item.sources?.includes('admin')).map((item)=>({actor,item})));
  const knowledge=subjects.flatMap((subject)=>subject.holders.filter((holder)=>holder.sources.includes('admin')).map((holder)=>({subject,holder})));
  return <details className="admin-claims"><summary>Administrative confirmations <span>{capabilities.length+knowledge.length}</span></summary><p className="profile-help">Removing a confirmation never removes the person’s own declaration or imported evidence.</p><div className="admin-claim-list">{capabilities.map(({actor,item})=><article key={`cap-${actor.id}-${item.id}`}><div><strong>{item.name}</strong><small>{actor.displayName} · {item.capabilityType}</small></div><button className="secondary" disabled={busy} onClick={()=>{if(window.confirm(`Remove the administrator confirmation for ${item.name} from ${actor.displayName}?`))perform(()=>onRemoveCapability(actor.id,item.id))}}>Remove confirmation</button></article>)}{knowledge.map(({subject,holder})=><article key={`knowledge-${holder.id}-${subject.id}`}><div><strong>{subject.name}</strong><small>{holder.displayName} · {subject.subjectType}</small></div><button className="secondary" disabled={busy} onClick={()=>{if(window.confirm(`Remove the administrator confirmation for ${subject.name} from ${holder.displayName}?`))perform(()=>onRemoveKnowledge(holder.id,subject.id))}}>Remove confirmation</button></article>)}{capabilities.length+knowledge.length===0&&<p className="muted">No administrative confirmations to review.</p>}</div></details>
}

function KnowledgeSubjects({ subjects, actors, canAdminister, busy, perform, onCreate, onAssign,onRemove }: { subjects: KnowledgeSubject[]; actors: Actor[]; canAdminister: boolean; busy: boolean; perform: (action: () => Promise<void>) => Promise<void>; onCreate: (name: string, type: KnowledgeSubject['subjectType']) => Promise<void>; onAssign: (actorId: string, subjectId: string) => Promise<void>;onRemove:(actorId:string,subjectId:string)=>Promise<void> }) {
  const [name,setName]=useState(''); const [type,setType]=useState<KnowledgeSubject['subjectType']>('service'); const [actorBySubject,setActorBySubject]=useState<Record<string,string>>({});
  return <section className="knowledge-section"><div className="knowledge-heading"><div><p className="eyebrow">Continuity</p><h3>Knowledge subjects</h3></div><p>Services, projects, and systems whose knowledge must not remain with only one person.</p></div>{canAdminister && <form className="knowledge-create" onSubmit={(event)=>{event.preventDefault(); if(!name.trim())return; perform(()=>onCreate(name.trim(),type)).then(()=>setName(''));}}><input aria-label="Knowledge subject name" placeholder="e.g. Payments API" value={name} onChange={(event)=>setName(event.target.value)} /><select aria-label="Knowledge subject type" value={type} onChange={(event)=>setType(event.target.value as KnowledgeSubject['subjectType'])}><option value="service">Service</option><option value="project">Project</option><option value="system">System</option><option value="domain">Domain</option><option value="other">Other</option></select><button disabled={busy}>Add entity</button></form>}<div className="knowledge-grid">{subjects.map((subject)=>{const available=actors.filter((actor)=>!subject.holders.some((holder)=>holder.id===actor.id)); const risk=subject.holders.length===0?'uncovered':subject.holders.length===1?'concentrated':'covered'; return <article className="knowledge-card" key={subject.id}><header><span className={`knowledge-type ${subject.subjectType}`}>{subject.subjectType}</span><span className={`knowledge-risk ${risk}`}>{risk==='uncovered'?'No knowledge owner':risk==='concentrated'?'Single point of failure':'Knowledge distributed'}</span></header><h4>{subject.name}</h4><div className="knowledge-holders">{subject.holders.map((holder)=><span key={holder.id}>{holder.displayName}</span>)}{subject.holders.length===0&&<small>No knowledgeable people recorded.</small>}</div>{canAdminister&&available.length>0&&<div className="assign-knowledge"><select aria-label={`Knowledge holder for ${subject.name}`} value={actorBySubject[subject.id]??''} onChange={(event)=>setActorBySubject((values)=>({...values,[subject.id]:event.target.value}))}><option value="">Add knowledgeable person…</option>{available.map((actor)=><option key={actor.id} value={actor.id}>{actor.displayName}</option>)}</select><button disabled={busy||!actorBySubject[subject.id]} onClick={()=>perform(()=>onAssign(actorBySubject[subject.id],subject.id)).then(()=>setActorBySubject((values)=>({...values,[subject.id]:''})))}>Assign</button></div>}</article>})}{subjects.length===0&&<p className="muted">Add the first service, project, system, or knowledge domain.</p>}</div></section>;
}

function ActorCard({ actor, capabilities, busy, currentWorkspaceRole, canAdminister, onAssign,onRemove,onUpdateRole,onRemoveMember }: { actor: Actor; capabilities: Capability[]; busy: boolean; currentWorkspaceRole:Directory['currentWorkspaceRole'];canAdminister: boolean; onAssign: (capabilityId: string) => Promise<void>;onRemove:(capabilityId:string)=>Promise<void>;onUpdateRole:(role:'admin'|'member')=>Promise<void>;onRemoveMember:()=>Promise<void> }) {
  const available = capabilities.filter((capability) => !actor.capabilities.some((owned) => owned.id === capability.id));
  const [selected, setSelected] = useState('');
  const canManageMembership=canAdminister&&actor.hasAccount&&!actor.isCurrent&&actor.workspaceRole!=='owner'&&(currentWorkspaceRole==='owner'||actor.workspaceRole==='member');
  return <article className="actor-card"><div className="actor-avatar">{actor.actorType === 'automation' ? '⚙' : actor.displayName.slice(0, 1).toUpperCase()}</div><div className="actor-copy"><div className="actor-name"><strong>{actor.displayName}{actor.isCurrent&&<small> · you</small>}</strong>{actor.hasAccount && <span>{actor.workspaceRole??'account'}</span>}</div>{canManageMembership&&<div className="member-access"><label>Access<select aria-label={`Workspace access for ${actor.displayName}`} value={actor.workspaceRole} disabled={busy} onChange={(event)=>onUpdateRole(event.target.value as 'admin'|'member')}><option value="member">Member</option>{currentWorkspaceRole==='owner'&&<option value="admin">Administrator</option>}</select></label><button className="danger" disabled={busy} onClick={()=>{if(window.confirm(`Remove ${actor.displayName}'s access to this workspace? Their work history and skill records will be kept.`))onRemoveMember()}}>Remove access</button></div>}<div className="capability-chips">{actor.capabilities.map((capability) => <span key={capability.id} className={capability.capabilityType}>{capability.name}</span>)}{actor.capabilities.length === 0 && <small>No capabilities yet</small>}</div>{canAdminister && available.length > 0 && <div className="assign-capability"><select aria-label={`Capability for ${actor.displayName}`} value={selected} onChange={(event) => setSelected(event.target.value)}><option value="">Add capability…</option>{available.map((capability) => <option key={capability.id} value={capability.id}>{capability.name} · {capability.capabilityType}</option>)}</select><button disabled={busy || !selected} onClick={() => onAssign(selected).then(() => setSelected(''))}>Assign</button></div>}</div></article>;
}

function InvitationManager({ workspaceId, busy }: { workspaceId: string; busy: boolean }) {
  const [items, setItems] = useState<Invitation[]>([]); const [name, setName] = useState(''); const [email, setEmail] = useState(''); const [role, setRole] = useState<Invitation['workspaceRole']>('member'); const [link, setLink] = useState(''); const [error, setError] = useState('');
  const load = () => api<Invitation[]>(`/api/v1/workspaces/${workspaceId}/invitations`).then(setItems).catch((cause: Error) => setError(cause.message));
  useEffect(() => { load(); }, [workspaceId]);
  async function submit(event: FormEvent) { event.preventDefault(); setError(''); setLink(''); try { const result = await api<{ invitation: Invitation; token: string }>(`/api/v1/workspaces/${workspaceId}/invitations`, { method: 'POST', body: JSON.stringify({ displayName: name, email, workspaceRole: role }) }); setLink(`${window.location.origin}${window.location.pathname}?invite=${encodeURIComponent(result.token)}`); setName(''); setEmail(''); await load(); } catch (cause) { setError(cause instanceof Error ? cause.message : 'Could not create invitation'); } }
  async function copyLink() { await navigator.clipboard.writeText(link); }
  return <section className="invitation-manager"><div className="section-title"><div><p className="eyebrow">Access</p><h3>Invite people</h3></div><span>{items.filter((item) => !item.acceptedAt).length}</span></div><form className="invite-form" onSubmit={submit}><input aria-label="Invitee name" placeholder="Full name" value={name} required onChange={(event) => setName(event.target.value)} /><input aria-label="Invitee email" type="email" placeholder="name@example.com" value={email} required onChange={(event) => setEmail(event.target.value)} /><select aria-label="Workspace role" value={role} onChange={(event) => setRole(event.target.value as Invitation['workspaceRole'])}><option value="member">Member</option><option value="admin">Administrator</option></select><button disabled={busy}>Create invite link</button></form>{error && <div className="auth-error">{error}</div>}{link && <div className="invite-link"><input readOnly value={link} aria-label="Invitation link" /><button onClick={copyLink}>Copy link</button><small>This link is shown once and expires in 7 days.</small></div>}<div className="invitation-list">{items.map((item) => <div key={item.id}><span className={`invite-state ${item.acceptedAt ? 'accepted' : ''}`}>{item.acceptedAt ? 'joined' : new Date(item.expiresAt) < new Date() ? 'expired' : 'pending'}</span><strong>{item.displayName}</strong><span>{item.email}</span><small>{item.workspaceRole}</small></div>)}{items.length === 0 && <p className="muted">No invitations yet.</p>}</div></section>;
}

function CreateDialog({ kind, parent, busy, firstRun=false, onClose, onSubmit }: { kind: 'workspace' | 'root' | 'child'; parent: WorkNode | null; busy: boolean; firstRun?:boolean;onClose: () => void; onSubmit: (name: string, outcome: string) => void }) {
  const [name, setName] = useState('');
  const [outcome, setOutcome] = useState('');
  const title = kind === 'workspace' ? firstRun?'Create your first work tree':'Create a workspace' : kind === 'root' ? 'Create a root goal' : `Add work under “${parent?.title}”`;
  function submit(event: FormEvent) { event.preventDefault(); onSubmit(name.trim(), outcome.trim()); }
  return <div className="dialog-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
    <section className={`dialog ${firstRun?'first-run-dialog':''}`} role="dialog" aria-modal="true" aria-labelledby="dialog-title"><button className="dialog-close" onClick={onClose}>×</button><p className="eyebrow">{firstRun?'Welcome to Work Graph':'Work Graph'}</p><h2 id="dialog-title">{title}</h2>{firstRun&&<p className="dialog-intro">Start with one meaningful outcome. Every task you add later will stay connected to this purpose.</p>}
      <form onSubmit={submit}>
        <label>{kind === 'workspace' ? 'Workspace name' : 'Title'}<input autoFocus value={name} placeholder={kind==='workspace'?'Personal projects, My company…':undefined} maxLength={kind === 'workspace' ? 200 : 500} required onChange={(event) => setName(event.target.value)} /></label>
        <label>{kind === 'workspace' ? 'What do you want to achieve?' : 'Desired outcome'}{kind === 'workspace' ? <input value={outcome} placeholder="Build a house, launch a product…" maxLength={500} required onChange={(event) => setOutcome(event.target.value)} /> : <textarea value={outcome} rows={4} onChange={(event) => setOutcome(event.target.value)} />}</label>
        <div className="dialog-actions"><button type="button" className="secondary" onClick={onClose}>{firstRun?'Do this later':'Cancel'}</button><button disabled={busy}>{busy ? 'Creating…' : firstRun?'Create my tree':'Create'}</button></div>
      </form>
    </section>
  </div>;
}

createRoot(document.getElementById('root')!).render(<StrictMode><App /></StrictMode>);
