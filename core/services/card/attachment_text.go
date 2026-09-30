package card

// Text attachments as editable documents.
//
// An attachment is a file the card owns in the vault; a workspace file is a
// file the user owns in a folder BRUV was pointed at. Same bytes-on-the-
// vault-host situation, different owner — so the document editor opens
// both through one DocumentSource contract (open → content + stamp, stat →
// stamp, save guarded by the stamp). These are the attachment half.

import (
	"fmt"
	"os"
	"unicode/utf8"

	"bruv/internal/fsutil"
	"bruv/internal/model"
)

// MaxTextAttachmentBytes caps what the editor will open — the same limit
// as workspace files. Bigger attachments stay download-only.
const MaxTextAttachmentBytes = 10 << 20

// OpenAttachmentText returns a text attachment's content plus the stamp
// the editor hands back to SaveAttachmentText. Binary or oversize
// attachments are refused so the caller falls back to download/preview.
func (s *Service) OpenAttachmentText(cardID, attachmentID string) (*model.WorkspaceFileContent, error) {
	raw, info, err := s.readAttachmentText(cardID, attachmentID)
	if err != nil {
		return nil, err
	}
	return &model.WorkspaceFileContent{Content: string(raw), Stamp: model.NewFileStamp(raw, info.ModTime())}, nil
}

// StatAttachmentText is the editor's external-change check.
func (s *Service) StatAttachmentText(cardID, attachmentID string) (*model.WorkspaceFileStamp, error) {
	raw, info, err := s.readAttachmentText(cardID, attachmentID)
	if err != nil {
		return nil, err
	}
	st := model.NewFileStamp(raw, info.ModTime())
	return &st, nil
}

// SaveAttachmentText is the guarded write: when expectedHash is set and
// the file no longer carries it, nothing is written and the result reports
// Diverged with the current stamp. The card's attachment record picks up
// the new size, and card:updated fires so other surfaces refresh.
func (s *Service) SaveAttachmentText(cardID, attachmentID, content, expectedHash string) (*model.WorkspaceSaveResult, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	if expectedHash != "" {
		current, err := s.StatAttachmentText(cardID, attachmentID)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if current != nil && current.Hash != expectedHash {
			return &model.WorkspaceSaveResult{Diverged: true, Stamp: *current}, nil
		}
	}
	if _, err := r.FindAttachment(cardID, attachmentID); err != nil {
		return nil, err
	}
	path := r.AttachmentPath(cardID, attachmentID)
	if err := fsutil.WriteFileAtomic(path, []byte(content), 0o644); err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	card, err := r.UpdateCard(cardID, func(c *model.Card) {
		for i := range c.FileAttachments {
			if c.FileAttachments[i].ID == attachmentID {
				c.FileAttachments[i].Size = info.Size()
			}
		}
	})
	if err != nil {
		return nil, err
	}
	s.emitCardUpdated(card)
	return &model.WorkspaceSaveResult{Stamp: model.NewFileStamp([]byte(content), info.ModTime())}, nil
}

func (s *Service) readAttachmentText(cardID, attachmentID string) ([]byte, os.FileInfo, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, nil, fmt.Errorf("no repository open")
	}
	att, err := r.FindAttachment(cardID, attachmentID)
	if err != nil {
		return nil, nil, err
	}
	path := r.AttachmentPath(cardID, attachmentID)
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	if info.Size() > MaxTextAttachmentBytes {
		return nil, nil, fmt.Errorf("%s is too large to open in BRUV", att.Name)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	if !utf8.Valid(raw) {
		return nil, nil, fmt.Errorf("%s is not a text file", att.Name)
	}
	return raw, info, nil
}
