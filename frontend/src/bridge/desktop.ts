import { Events } from '@wailsio/runtime';
import {
  Error as NativeError,
  Information,
  TakeReview,
} from '../../bindings/github.com/shdeen/saferm/cmd/saferm-app/windowservice';

/** Shows a message in a native error dialog attached to the SafeRM window. */
export const showError = NativeError;
/** Shows a message in a native information dialog attached to the SafeRM window. */
export const showInformation = Information;
/**
 * Returns the ID of the action that a launch of the application asked to review, or
 * zero when there is none. Each ID is returned only once.
 */
export const takeReview = TakeReview;
/**
 * Calls the handler each time another launch activates the running application.
 * Returns the function that unsubscribes the handler.
 */
export function onReview(handler: () => void): () => void {
  return Events.On('saferm:review', handler);
}
