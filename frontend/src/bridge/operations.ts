import {
  List,
  Run,
  Statuses,
  UpdateSettings,
  DirectoryExists,
  RuleMalformed,
  OpenConfiguration,
} from '../../bindings/github.com/shdeen/saferm/cmd/saferm-app/operationservice';
import {
  Source,
  Destination,
  Directory,
} from '../../bindings/github.com/shdeen/saferm/cmd/saferm-app/filedialogs';
import type { Request } from '../../bindings/github.com/shdeen/saferm/internal/saferm/models';
import type { Settings } from '../../bindings/github.com/shdeen/saferm/internal/config/models';

/**
 * The most recently queued configuration request. Every configuration read, save, and
 * reset waits for it, so they run in the order requested, even across navigation.
 */
let configurationRequest: Promise<unknown> = Promise.resolve();

/** Operation request, result, and confirmation types generated from Go. */
export type {
  Request,
  Result,
  Confirmation,
} from '../../bindings/github.com/shdeen/saferm/internal/saferm/models';
/** History record of one delete or move, generated from Go. */
export type { Action } from '../../bindings/github.com/shdeen/saferm/internal/store/models';
/** History page and operation response types that the desktop service returns. */
export type {
  ActionPage,
  OperationResponse,
} from '../../bindings/github.com/shdeen/saferm/cmd/saferm-app/models';
/** Configuration values shared with the command-line application. */
export type { Settings } from '../../bindings/github.com/shdeen/saferm/internal/config/models';

/** Returns one page of the actions that match the type and status filters. */
export const getOperationPage = List;
/** Returns each status code keyed by its status name. */
export const getOperationStatuses = Statuses;
/**
 * Prompts for an existing file or directory. Returns the selected path, or an empty
 * string on cancellation.
 */
export const chooseSourcePath = Source;
/**
 * Prompts for a new path, proposing the base name of the supplied source path. Returns the
 * selected path, or an empty string on cancellation.
 */
export const chooseDestinationPath = Destination;
/**
 * Prompts for an existing directory, starting in the supplied directory when it exists.
 * Returns the selected path, or an empty string on cancellation.
 */
export const chooseDirectory = Directory;
/** Reports whether the path names an existing directory. */
export const directoryExists: (directoryPath: string) => Promise<boolean> = DirectoryExists;
/** Reports whether the review pattern is malformed. */
export const ruleMalformed: (pattern: string) => Promise<boolean> = RuleMalformed;
/** Opens the configuration file in its associated application. */
export const openConfiguration = OpenConfiguration;

/**
 * Saves the supplied settings and leaves the omitted settings unchanged. Runs after
 * every configuration request that was made earlier.
 */
export function updateSettings(changes: Partial<Settings>) {
  const saveRequest = configurationRequest.then(function saveConfiguration() {
    return UpdateSettings({
      trashPath: null,
      retainPeriod: null,
      requireReview: null,
      path: '',
      external: false,
      ...changes,
    });
  });
  configurationRequest = saveRequest.catch(function allowNextRequest() {});
  return saveRequest;
}

/**
 * Runs a request through the desktop service, giving omitted fields their empty values.
 * A config request runs after every configuration request that was made earlier.
 */
export function runOperation(request: Partial<Request>) {
  function executeRequest() {
    return Run({
      ExtraArguments: [],
      Command: '',
      Source: '',
      Destination: '',
      Reason: '',
      Requester: '',
      Description: '',
      ID: '',
      IDs: [],
      Decision: '',
      Type: '',
      Status: '',
      Overwrite: false,
      Force: false,
      Orphans: false,
      Default: false,
      Setup: false,
      Reset: false,
      Confirmed: false,
      ...request,
    });
  }
  if (request.Command !== 'config') return executeRequest();
  const configRequest = configurationRequest.then(executeRequest);
  configurationRequest = configRequest.catch(function allowNextRequest() {});
  return configRequest;
}
