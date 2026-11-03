# Separate locking from state replacement

One-shot runs from the timer and interface hooks may overlap. Hold an operating-system lock on a stable file beside the state file for the whole read, update, notification, and state-write sequence. Replace the state file atomically so a crash does not leave a partially written file. Locking the state file itself would not protect its replacement: another process could lock the new file while the first still holds the old one. The operating system releases the lock when the holder exits, including when killed; the lock file may remain on disk.

A missing state file forces updates. Corrupt or unreadable state is an error, not a reason to overwrite potentially useful evidence. Notification state is persisted for retries; submission is at least once because a crash between local MTA acceptance and recording it can cause a duplicate.
