-- Direct coordination 0.3: orders, runs and events only.
CREATE SCHEMA IF NOT EXISTS hev_03;
CREATE TABLE hev_03.orders (
 id text PRIMARY KEY, brief jsonb NOT NULL, revision bigint NOT NULL DEFAULT 1,
 owner text, generation bigint NOT NULL DEFAULT 0, lease_until timestamptz,
 status text NOT NULL DEFAULT 'open' CHECK(status IN ('open','blocked','done','cancelled'))
);
CREATE TABLE hev_03.runs (
 id text PRIMARY KEY, order_id text NOT NULL REFERENCES hev_03.orders(id),
 revision bigint NOT NULL, dispatch_key text NOT NULL, assignment jsonb NOT NULL,
 node_id text NOT NULL, incarnation text NOT NULL,
 status text NOT NULL DEFAULT 'requested' CHECK(status IN ('requested','running','succeeded','failed','cancelled','unknown')),
 runtime_id text, result jsonb, created timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(order_id,dispatch_key)
);
CREATE TABLE hev_03.events (
 sequence bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 at timestamptz NOT NULL DEFAULT clock_timestamp(), actor text NOT NULL,
 resource text NOT NULL, kind text NOT NULL, record jsonb NOT NULL
);
CREATE FUNCTION hev_03.change(p_action text, p jsonb) RETURNS jsonb
LANGUAGE plpgsql SET search_path=pg_catalog,hev_03 AS $$
DECLARE o hev_03.orders; r hev_03.runs; output jsonb;
BEGIN
 IF p_action='create' THEN
  INSERT INTO hev_03.orders(id,brief) VALUES(p->>'id',p->'brief')
   ON CONFLICT(id) DO NOTHING;
  SELECT * INTO STRICT o FROM hev_03.orders WHERE id=p->>'id' FOR UPDATE;
  IF o.brief IS DISTINCT FROM p->'brief' THEN RAISE EXCEPTION 'create key conflict'; END IF;
  output=to_jsonb(o);

 ELSE
  SELECT * INTO STRICT o FROM hev_03.orders WHERE id=p->>'order' FOR UPDATE;
  IF p_action='claim' THEN
   IF o.status IN ('done','cancelled') OR (o.lease_until>clock_timestamp() AND o.owner IS DISTINCT FROM p->>'actor') THEN RAISE EXCEPTION 'order unavailable'; END IF;
   IF (p->>'ttl')::int NOT BETWEEN 1 AND 300 THEN RAISE EXCEPTION 'ttl outside 1..300'; END IF;
   UPDATE hev_03.orders SET owner=p->>'actor',generation=generation+1,lease_until=clock_timestamp()+make_interval(secs=>(p->>'ttl')::int) WHERE id=o.id RETURNING * INTO o;
   output=to_jsonb(o);
  ELSIF p_action IN ('assign','revise','settle','accept','reconcile') THEN
   IF o.owner IS DISTINCT FROM p->>'actor' OR o.generation IS DISTINCT FROM (p->>'generation')::bigint OR o.revision IS DISTINCT FROM (p->>'revision')::bigint OR o.lease_until<=clock_timestamp() OR o.lease_until IS NULL THEN RAISE EXCEPTION 'stale ownership or revision'; END IF;
   IF p_action='assign' THEN
    SELECT * INTO r FROM hev_03.runs WHERE order_id=o.id AND dispatch_key=p->>'key';
    IF FOUND THEN
     IF r.assignment IS DISTINCT FROM p->'assignment' OR r.node_id IS DISTINCT FROM p->>'node' OR r.incarnation IS DISTINCT FROM p->>'incarnation' THEN RAISE EXCEPTION 'dispatch key conflict'; END IF;
     RETURN to_jsonb(r);
    END IF;
    IF o.status<>'open' THEN RAISE EXCEPTION 'order is not open'; END IF;
    IF EXISTS(SELECT 1 FROM hev_03.runs WHERE order_id=o.id AND status IN ('requested','running','unknown')) THEN RAISE EXCEPTION 'reconcile existing run before assigning'; END IF;
    INSERT INTO hev_03.runs(id,order_id,revision,dispatch_key,assignment,node_id,incarnation) VALUES(p->>'run',o.id,o.revision,p->>'key',p->'assignment',p->>'node',p->>'incarnation') RETURNING * INTO r;
    output=to_jsonb(r);
   ELSIF p_action='accept' THEN
    SELECT * INTO STRICT r FROM hev_03.runs WHERE id=p->>'run' AND order_id=o.id FOR UPDATE;
    IF r.revision<>o.revision OR r.status<>'succeeded' OR (r.assignment->>'role') IS DISTINCT FROM 'validation' OR r.assignment->>'source_revision' IS DISTINCT FROM p->>'candidate' OR r.result->>'candidate' IS DISTINCT FROM p->>'candidate' OR coalesce(p->>'evidence','')='' THEN RAISE EXCEPTION 'candidate-bound validation evidence required'; END IF;
    IF NOT EXISTS(SELECT 1 FROM hev_03.runs i WHERE i.id=r.assignment->>'implementation_run' AND i.order_id=o.id AND i.revision=o.revision AND i.assignment->>'role'='implementation' AND i.status='succeeded' AND i.result->>'candidate'=p->>'candidate' AND i.runtime_id IS DISTINCT FROM r.runtime_id AND i.runtime_id IS NOT NULL AND r.runtime_id IS NOT NULL) THEN RAISE EXCEPTION 'independent implementation result required'; END IF;
    IF r.result->>'passed'='true' THEN
     IF r.result->>'evidence' IS DISTINCT FROM p->>'evidence' THEN RAISE EXCEPTION 'acceptance receipt is immutable'; END IF;
     RETURN to_jsonb(r);
    END IF;
    UPDATE hev_03.runs SET result=result||jsonb_build_object('passed',true,'evidence',p->>'evidence','inspector',p->>'actor') WHERE id=r.id RETURNING * INTO r;
    output=to_jsonb(r);
   ELSIF p_action='reconcile' THEN
    SELECT * INTO STRICT r FROM hev_03.runs WHERE id=p->>'run' AND order_id=o.id FOR UPDATE;
    IF r.status<>'unknown' OR coalesce(p->>'evidence','')='' OR coalesce(p->>'candidate','')='' THEN RAISE EXCEPTION 'unknown run and inspected evidence/candidate required'; END IF;
    UPDATE hev_03.runs SET status='failed',result=coalesce(result,'{}'::jsonb)||jsonb_build_object('candidate',p->>'candidate','inspection',p->>'evidence','inspector',p->>'actor','disposition','inspected failed attempt') WHERE id=r.id RETURNING * INTO r;
    output=to_jsonb(r);
   ELSIF p_action='revise' THEN
    IF EXISTS(SELECT 1 FROM hev_03.runs WHERE order_id=o.id AND status IN ('requested','running','unknown')) THEN RAISE EXCEPTION 'active effects must be reconciled'; END IF;
    UPDATE hev_03.orders SET brief=p->'brief',revision=revision+1 WHERE id=o.id RETURNING * INTO o;
    output=to_jsonb(o);
   ELSE
    IF p->>'status'='done' THEN
     IF NOT EXISTS(SELECT 1 FROM hev_03.runs WHERE order_id=o.id AND revision=o.revision AND status='succeeded' AND assignment->>'role'='validation' AND assignment->>'source_revision'=p->>'candidate' AND result->>'candidate'=p->>'candidate' AND result->>'passed'='true' AND coalesce(result->>'evidence','')<>'' AND coalesce(result->>'inspector','')<>'') THEN RAISE EXCEPTION 'matching independent acceptance required'; END IF;
    END IF;
    IF EXISTS(SELECT 1 FROM hev_03.runs WHERE order_id=o.id AND status IN ('requested','running','unknown')) THEN RAISE EXCEPTION 'active effects must be reconciled'; END IF;
    UPDATE hev_03.orders SET status=p->>'status' WHERE id=o.id RETURNING * INTO o;
    output=to_jsonb(o);
   END IF;
  ELSE
   -- Execution publication is authorized by the immutable node/incarnation,
   -- independently of the FOH lease. Never grant it an order mutation.
   SELECT * INTO STRICT r FROM hev_03.runs WHERE id=p->>'run' AND order_id=o.id FOR UPDATE;
   IF r.node_id IS DISTINCT FROM p->>'node' OR r.incarnation IS DISTINCT FROM p->>'incarnation' THEN RAISE EXCEPTION 'execution identity mismatch'; END IF;
   IF p_action='start' THEN
    IF r.status='running' THEN
     IF r.runtime_id IS DISTINCT FROM p->>'runtime_id' THEN RAISE EXCEPTION 'runtime identity conflict'; END IF;
     RETURN to_jsonb(r);
    END IF;
    IF r.status<>'requested' THEN RAISE EXCEPTION 'run is not executable'; END IF;
    UPDATE hev_03.runs SET status='running',runtime_id=p->>'runtime_id' WHERE id=r.id RETURNING * INTO r;
   ELSIF p_action='report' THEN
    IF coalesce(p->'result'->>'passed','false')<>'false' OR (p->'result') ? 'evidence' OR (p->'result') ? 'inspector' THEN RAISE EXCEPTION 'execution report cannot grant acceptance'; END IF;
    IF r.status IN ('succeeded','failed','cancelled') THEN
     IF r.result IS DISTINCT FROM p->'result' OR r.status IS DISTINCT FROM p->>'status' THEN RAISE EXCEPTION 'result conflict'; END IF;
     RETURN to_jsonb(r);
    END IF;
    IF p->>'status' IS NULL OR p->>'status' NOT IN ('succeeded','failed','unknown') THEN RAISE EXCEPTION 'invalid execution result'; END IF;
    IF r.status='unknown' AND p->>'status'<>'unknown' AND coalesce(p->>'inspection','')='' THEN RAISE EXCEPTION 'unknown requires inspection evidence'; END IF;
    UPDATE hev_03.runs SET status=p->>'status',result=p->'result' WHERE id=r.id RETURNING * INTO r;
   ELSE RAISE EXCEPTION 'unsupported mutation'; END IF;
   output=to_jsonb(r);
  END IF;
 END IF;
 INSERT INTO hev_03.events(actor,resource,kind,record) VALUES(coalesce(p->>'actor',p->>'node'),coalesce(p->>'order',p->>'id'),p_action,output);
 RETURN output;
END $$;
