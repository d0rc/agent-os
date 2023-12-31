package agency

import "time"

const NumberOfVotesToCache = 3
const VoterMinResults = 3
const MinimumNumberOfVotes = 3
const MinimalVotingRatingForCommand = 1.0
const ToTPathLenToTriggerTerminalCallback = 18
const ResubmitSystemPromptAfter = 15 * time.Minute
const MaxIoRequestsThreads = 192
const WriteVotesLog = true
