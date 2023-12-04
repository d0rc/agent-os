package agency

import "time"

const NumberOfVotesToCache = 5
const VoterMinResults = 7
const MinimalVotingRatingForCommand = 6.0
const ToTPathLenToTriggerTerminalCallback = 7
const ResubmitSystemPromptAfter = 1 * time.Minute
