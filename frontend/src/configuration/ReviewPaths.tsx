import { useEffect, useRef, useState } from 'react';
import { ActionIcon, Group, TextInput } from '@mantine/core';
import { Minus, Plus } from 'lucide-react';
import { ruleMalformed } from '../bridge/operations';
import { appstrings } from '../shared/text';

/** One row of the list. */
type ReviewRow = {
  /** A number that identifies the row for as long as the list is shown. */
  rowID: number;
  /** The text in the row's field. */
  text: string;
  /** Whether the text in the field is malformed, as last checked. */
  malformed: boolean;
  /** Whether the row's pattern in the saved list is malformed. Only a pattern from the initial list can be. */
  savedMalformed: boolean;
};

/** A list handed to onChange that is not saved yet, with the row behind each of its patterns. */
type ReportedList = {
  paths: string[];
  rowIDs: number[];
};

/**
 * Edits the ordered list of review patterns. The initial list and every edit are
 * checked for malformed patterns, and the accepted list is reported when it differs
 * from the saved list. A row whose text is malformed keeps that text, contributes its
 * saved pattern to the reported list, and shows a message while focus is elsewhere.
 * A malformed pattern in the initial list also shows a message for the whole list.
 */
export function ReviewPaths({
  initialPaths,
  savedPaths,
  disabled,
  onChange,
  onError,
  onEdit,
  onValidation,
  awaitSavedPaths,
}: {
  initialPaths: string[];
  /**
   * The list as last saved. A list handed to onChange counts as saved once that very
   * list arrives here or from awaitSavedPaths.
   */
  savedPaths: string[];
  disabled: boolean;
  /** Receives the accepted list when it differs from the saved list. */
  onChange: (paths: string[]) => void;
  onError: (error: string) => void;
  /** Called on every edit, before the edited rows are checked. */
  onEdit: () => void;
  /** Receives a promise that settles when the check of an edit finishes. */
  onValidation: (validation: Promise<void>) => void;
  /** Waits for every save that has started and returns the list as last saved. */
  awaitSavedPaths: () => Promise<string[]>;
}) {
  const [reviewRows, setReviewRows] = useState<ReviewRow[]>(function initialRows() {
    return initialPaths.map(function initialRow(path, index) {
      return { rowID: index, text: path, malformed: false, savedMalformed: false };
    });
  });
  const [selectedIndex, setSelectedIndex] = useState<number>();
  const [focusedIndex, setFocusedIndex] = useState<number>();
  const inputElements = useRef<(HTMLInputElement | null)[]>([]);
  const savedTexts = useRef(new Map<number, string>(Array.from(initialPaths.entries())));
  const reportedLists = useRef<ReportedList[]>([]);
  const nextRowID = useRef(initialPaths.length);
  const checkRequest = useRef(0);
  const mounted = useRef(true);

  /**
   * Records the saved list when it is one of the reported lists: each of its rows
   * takes its pattern in that list as its saved pattern, and a row whose saved
   * pattern changed no longer counts as holding a malformed saved pattern.
   */
  function recordSavedList(savedList: string[]) {
    let savedIndex = reportedLists.current.length - 1;
    while (savedIndex >= 0 && reportedLists.current[savedIndex].paths !== savedList) {
      savedIndex--;
    }
    if (savedIndex < 0) return;
    const reportedList = reportedLists.current[savedIndex];
    reportedLists.current = reportedLists.current.slice(savedIndex + 1);
    const resavedRowIDs = reportedList.rowIDs.filter(function savedTextChanged(rowID, position) {
      return savedTexts.current.get(rowID) !== reportedList.paths[position];
    });
    reportedList.rowIDs.forEach(function saveText(rowID, position) {
      savedTexts.current.set(rowID, reportedList.paths[position]);
    });
    if (!mounted.current) return;
    setReviewRows(function clearSavedMarks(currentRows) {
      return currentRows.map(function clearSavedMark(reviewRow) {
        return resavedRowIDs.includes(reviewRow.rowID)
          ? { ...reviewRow, savedMalformed: false }
          : reviewRow;
      });
    });
  }

  /**
   * Shows the rows and asks whether each row's text is malformed. Once every save
   * that has started is finished, a row with malformed text contributes its saved
   * pattern to the accepted list, and the accepted list is reported when it differs
   * from the saved list. Does nothing more when a later check has started, and
   * changes no display once the component has unmounted.
   */
  async function checkRows(editedRows: ReviewRow[]) {
    setReviewRows(editedRows);
    const requestNumber = ++checkRequest.current;
    try {
      const malformedAnswers = await Promise.all(
        editedRows.map(function checkRow(reviewRow) {
          return ruleMalformed(reviewRow.text);
        }),
      );
      if (requestNumber !== checkRequest.current) return;
      if (mounted.current) {
        setReviewRows(function showChecks(currentRows) {
          return currentRows.map(function showCheck(reviewRow, index) {
            return {
              ...reviewRow,
              malformed: malformedAnswers[index],
              savedMalformed:
                reviewRow.savedMalformed ||
                (malformedAnswers[index] &&
                  reviewRow.text === savedTexts.current.get(reviewRow.rowID)),
            };
          });
        });
      }
      const savedList = await awaitSavedPaths();
      if (requestNumber !== checkRequest.current) return;
      recordSavedList(savedList);
      const acceptedPaths = editedRows.map(function acceptedPattern(reviewRow, index) {
        return malformedAnswers[index]
          ? (savedTexts.current.get(reviewRow.rowID) ?? '')
          : reviewRow.text;
      });
      const listChanged =
        acceptedPaths.length !== savedList.length ||
        acceptedPaths.some(function differs(path, index) {
          return path !== savedList[index];
        });
      if (!listChanged) return;
      reportedLists.current.push({
        paths: acceptedPaths,
        rowIDs: editedRows.map(function rowIdentity(reviewRow) {
          return reviewRow.rowID;
        }),
      });
      onChange(acceptedPaths);
    } catch (error) {
      if (mounted.current) onError(String(error));
    }
  }

  /** Announces an edit, checks the edited rows, and hands on the promise of that check. */
  function editRows(editedRows: ReviewRow[]) {
    onEdit();
    onValidation(checkRows(editedRows));
  }

  useEffect(
    function showSavedList() {
      recordSavedList(savedPaths);
    },
    [savedPaths],
  );

  useEffect(function checkInitialRows() {
    mounted.current = true;
    void checkRows(reviewRows);
    return function cancelCheck() {
      mounted.current = false;
    };
    // The initial list is checked once, when the component mounts.
  }, []);

  return (
    <div role="group" aria-labelledby="review-paths-label">
      <div className="settings-label">
        <span id="review-paths-label">{appstrings.reviewRules}</span>
        <span className="settings-validation invalid" role="status">
          {reviewRows.some(function savedMalformed(reviewRow) {
            return reviewRow.savedMalformed;
          })
            ? appstrings.malformedRule
            : ''}
        </span>
      </div>
      <div className="review-path-list">
        <div className="review-path-inputs">
          {reviewRows.length === 0 && (
            <div className="empty-review-paths">{appstrings.noRules}</div>
          )}
          {reviewRows.map(function reviewPathInput(reviewRow, index) {
            const showsInvalidRule = reviewRow.malformed && focusedIndex !== index;
            return (
              <div
                key={index}
                className={`review-path-row ${selectedIndex === index ? 'selected-review-path' : ''}`}
              >
                <TextInput
                  className="grow"
                  value={reviewRow.text}
                  disabled={disabled}
                  variant="unstyled"
                  aria-label={`${appstrings.reviewRule} ${index + 1}`}
                  aria-invalid={showsInvalidRule}
                  aria-describedby={`review-path-validation-${index}`}
                  ref={function rememberInput(element) {
                    inputElements.current[index] = element;
                  }}
                  onFocus={function selectPath() {
                    setSelectedIndex(index);
                    setFocusedIndex(index);
                  }}
                  onBlur={function leavePath() {
                    setFocusedIndex(undefined);
                  }}
                  onChange={function editPath(event) {
                    const retypedRows = [...reviewRows];
                    retypedRows[index] = { ...reviewRow, text: event.currentTarget.value };
                    editRows(retypedRows);
                  }}
                />
                <span
                  id={`review-path-validation-${index}`}
                  className="settings-validation invalid"
                  role="status"
                >
                  {showsInvalidRule ? appstrings.invalidRule : ''}
                </span>
              </div>
            );
          })}
        </div>
        <Group gap={0} className="review-path-actions">
          <ActionIcon
            variant="subtle"
            color="gray"
            aria-label={appstrings.addReviewPath}
            disabled={disabled}
            onClick={function addPath() {
              const addedIndex = reviewRows.length;
              editRows([
                ...reviewRows,
                { rowID: nextRowID.current++, text: '', malformed: false, savedMalformed: false },
              ]);
              setSelectedIndex(addedIndex);
              window.requestAnimationFrame(function focusAddedPath() {
                inputElements.current[addedIndex]?.focus();
              });
            }}
          >
            <Plus size={16} />
          </ActionIcon>
          <ActionIcon
            variant="subtle"
            color="gray"
            aria-label={appstrings.removeReviewPath}
            disabled={disabled || selectedIndex === undefined}
            onClick={function removePath() {
              editRows(
                reviewRows.filter(function retainRow(_reviewRow, index) {
                  return index !== selectedIndex;
                }),
              );
              setSelectedIndex(undefined);
            }}
          >
            <Minus size={16} />
          </ActionIcon>
        </Group>
      </div>
    </div>
  );
}
