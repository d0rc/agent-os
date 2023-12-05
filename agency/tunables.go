package agency

import "time"

const NumberOfVotesToCache = 3
const VoterMinResults = 7
const MinimumNumberOfVotes = 3
const MinimalVotingRatingForCommand = 5.0
const ToTPathLenToTriggerTerminalCallback = 7
const ResubmitSystemPromptAfter = 15 * time.Minute
const MaxIoRequestsThreads = 128
