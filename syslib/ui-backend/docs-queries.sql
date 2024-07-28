-- name: ddl-create-users
CREATE TABLE IF NOT EXISTS Users (
                                     UserID INT AUTO_INCREMENT PRIMARY KEY,
                                     Username VARCHAR(255) NOT NULL,
    PasswordHash VARCHAR(255) NOT NULL,
    UNIQUE(Username)
    ) ENGINE=InnoDB;

-- name: ddl-create-documents
CREATE TABLE IF NOT EXISTS Documents (
                                         DocID INT AUTO_INCREMENT PRIMARY KEY,
                                         UserID INT,
                                         Name VARCHAR(255) NOT NULL,
    UploadTime DATETIME DEFAULT CURRENT_TIMESTAMP,
    Status ENUM('processed', 'pending') NOT NULL,
    Progress FLOAT DEFAULT 0,
    Comment TEXT,
    ContentType VARCHAR(50),
    FOREIGN KEY (UserID) REFERENCES Users(UserID),
    INDEX idx_userid (UserID),
    INDEX idx_docname (Name),
    INDEX idx_docstatus (Status)
    ) ENGINE=InnoDB;

-- name: ddl-create-tags
CREATE TABLE IF NOT EXISTS Tags (
                                    TagID INT AUTO_INCREMENT PRIMARY KEY,
                                    TagName VARCHAR(50) NOT NULL,
    UNIQUE(TagName)
    ) ENGINE=InnoDB;

-- name: ddl-create-documenttags
CREATE TABLE IF NOT EXISTS DocumentTags (
                                            DocID INT,
                                            TagID INT,
                                            PRIMARY KEY (DocID, TagID),
    FOREIGN KEY (DocID) REFERENCES Documents(DocID),
    FOREIGN KEY (TagID) REFERENCES Tags(TagID),
    INDEX idx_docid (DocID),
    INDEX idx_tagid (TagID)
    ) ENGINE=InnoDB;

-- name: ddl-create-tokens
CREATE TABLE IF NOT EXISTS Tokens (
                                      TokenID INT AUTO_INCREMENT PRIMARY KEY,
                                      UserID INT,
                                      Token VARCHAR(255) NOT NULL,
    Expiry DATETIME NOT NULL,
    FOREIGN KEY (UserID) REFERENCES Users(UserID),
    UNIQUE(Token),
    INDEX idx_userid (UserID)
    ) ENGINE=InnoDB;

-- name: add-user
INSERT INTO Users (Username, PasswordHash) VALUES (?, ?);

-- name: authenticate-user
SELECT UserID, PasswordHash FROM Users WHERE Username = ?;

-- name: add-document
INSERT INTO Documents (UserID, Name, Status, Progress, Comment, ContentType) VALUES (?, ?, 'pending', 0, '', ?);

-- name: get-document-by-id
SELECT DocID, UserID, Name, UploadTime, Status, Progress, Comment, ContentType FROM Documents WHERE DocID = ?;

-- name: update-document-status
UPDATE Documents SET Status = ?, Progress = ?, Comment = ? WHERE DocID = ?;

-- name: add-tag
INSERT INTO Tags (TagName) VALUES (?);

-- name: get-tag-id
SELECT TagID FROM Tags WHERE TagName = ?;

-- name: associate-document-tag
INSERT INTO DocumentTags (DocID, TagID) VALUES (?, ?);

-- name: remove-document-tag
DELETE FROM DocumentTags WHERE DocID = ? AND TagID = ?;

-- name: get-documents-by-tag
SELECT d.DocID, d.UserID, d.Name, d.UploadTime, d.Status, d.Progress, d.Comment, d.ContentType
FROM Documents d
         JOIN DocumentTags dt ON d.DocID = dt.DocID
         JOIN Tags t ON dt.TagID = t.TagID
WHERE t.TagName = ?;

-- name: get-documents-by-name
SELECT DocID, UserID, Name, UploadTime, Status, Progress, Comment, ContentType FROM Documents WHERE Name LIKE ?;

-- name: search-documents-by-text
SELECT DocID, UserID, Name, UploadTime, Status, Progress, Comment, ContentType FROM Documents WHERE Comment LIKE ?;

-- name: add-token
INSERT INTO Tokens (UserID, Token, Expiry) VALUES (?, ?, ?);

-- name: get-token
SELECT Token, Expiry FROM Tokens WHERE Token = ?;

-- name: remove-token
DELETE FROM Tokens WHERE Token = ?;
