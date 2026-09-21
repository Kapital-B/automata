/**
 * What each object in the model actually is (spec §8.4). These are the
 * definitions the project page never gave, which is why confirming a fact and
 * the current position changing looked unrelated. Empty states carry the
 * definition rather than an apology.
 */
export const DEFINITIONS = {
  facts:
    "Facts are values that are currently true about this project. They are extracted from correspondence filed here, and supersede each other as things change.",
  decisions:
    "Decisions are choices this project has committed to. A new one waits for you to accept it, because asserting the project decided something is harder to walk back than a value being wrong.",
  issues:
    "Issues are open questions or work someone has to act on. They are raised from correspondence automatically — discard one that should not have been raised.",
  contradictions:
    "Contradictions are two sources disagreeing about the same value. Nothing is applied until you pick a side.",
  confirmation:
    "Anything the model could not apply on its own waits here — a value that would replace another, a decision it will not assert for you, or a disagreement it cannot settle.",
  position:
    "Derived from confirmed facts and accepted decisions. Confirm something below and it appears here.",
} as const;
