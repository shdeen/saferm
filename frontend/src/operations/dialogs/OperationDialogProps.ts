import type { Request } from '../../bridge/operations';

/** Props shared by the dialogs that submit an operation request. */
export type OperationDialogProps = {
  /** Whether an operation is running. */
  busy: boolean;
  onClose: () => void;
  /**
   * Submits a request. Resolves to true only when the operation finishes without an
   * error and without asking for confirmation.
   */
  perform: (request: Partial<Request>) => Promise<boolean>;
};
