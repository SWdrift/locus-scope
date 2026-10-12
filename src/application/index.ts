import { ApplicationService } from './service.js';
import type { Workspace } from '../domain/index.js';
import './graph.js';
import './diff.js';
import './mutation.js';
export { ApplicationService, cloneObject, cloneValue, compareEntityKey, entityKeyId, makeEntityRefs, makeScopeRefs } from './service.js';
export type * from './types.js';

export { ApplicationService as Service } from './service.js';
export { ApplicationService as WorkspaceApplication } from './service.js';
export function New(workspace: Workspace): ApplicationService {
  return new ApplicationService(workspace);
}
