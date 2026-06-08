-- Fix dispatch_plan column: it stores raw orchestrator text output, not JSON
ALTER TABLE orch_tasks ALTER COLUMN dispatch_plan TYPE TEXT USING dispatch_plan::text;
