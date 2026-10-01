package handlers_test

import (
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"demo_echo/internal/models"
	"demo_echo/internal/repository"

	"gorm.io/gorm"
)

// fakeDB là "database" trong bộ nhớ cho test handler. Các fake repository bên dưới
// cài đặt đúng interface trong internal/repository và mô phỏng hành vi GORM quan trọng:
// ErrRecordNotFound, ErrDuplicatedKey (unique index) và Preload Author/Tags/User.
type fakeDB struct {
	mu sync.Mutex

	users       map[uint]models.User
	articles    map[uint]models.Article
	tags        map[uint]models.Tag
	articleTags map[uint][]uint // article ID -> tag IDs
	comments    map[uint]models.Comment
	follows     map[[2]uint]bool // {follower, following}

	lastID uint
	clock  time.Time

	// beforeArticleInsert (nếu có) chạy ngay trước khi ghi article, sau khi handler đã
	// chọn slug: dùng để tái hiện race "request khác vừa chiếm đúng slug này".
	beforeArticleInsert func(db *fakeDB, article *models.Article)
}

func newFakeDB() *fakeDB {
	return &fakeDB{
		users:       map[uint]models.User{},
		articles:    map[uint]models.Article{},
		tags:        map[uint]models.Tag{},
		articleTags: map[uint][]uint{},
		comments:    map[uint]models.Comment{},
		follows:     map[[2]uint]bool{},
		clock:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func (db *fakeDB) nextID() uint {
	db.lastID++
	return db.lastID
}

// tick trả về thời điểm tăng dần để thứ tự created_at trong test là xác định.
func (db *fakeDB) tick() time.Time {
	db.clock = db.clock.Add(time.Second)
	return db.clock
}

func (db *fakeDB) repos() (repository.UserRepository, repository.ArticleRepository, repository.TagRepository, repository.CommentRepository, repository.FollowRepository) {
	return fakeUsers{db}, fakeArticles{db}, fakeTags{db}, fakeComments{db}, fakeFollows{db}
}

// ---------- users ----------

type fakeUsers struct{ db *fakeDB }

func (r fakeUsers) FindAll() ([]models.User, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	users := make([]models.User, 0, len(r.db.users))
	for _, u := range r.db.users {
		users = append(users, u)
	}
	sort.Slice(users, func(i, j int) bool { return users[i].ID > users[j].ID })
	return users, nil
}

func (r fakeUsers) FindByID(id uint) (*models.User, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	u, ok := r.db.users[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return &u, nil
}

func (r fakeUsers) findBy(match func(models.User) bool) (*models.User, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	for _, u := range r.db.users {
		if match(u) {
			return &u, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (r fakeUsers) FindByUsername(username string) (*models.User, error) {
	return r.findBy(func(u models.User) bool { return u.Username == username })
}

func (r fakeUsers) FindByEmail(email string) (*models.User, error) {
	return r.findBy(func(u models.User) bool { return u.Email == email })
}

func (r fakeUsers) duplicate(user *models.User) bool {
	for _, u := range r.db.users {
		if u.ID != user.ID && (u.Username == user.Username || u.Email == user.Email) {
			return true
		}
	}
	return false
}

func (r fakeUsers) Create(user *models.User) error {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	if r.duplicate(user) {
		return gorm.ErrDuplicatedKey
	}
	user.ID = r.db.nextID()
	user.CreatedAt = r.db.tick()
	user.UpdatedAt = user.CreatedAt
	r.db.users[user.ID] = *user
	return nil
}

func (r fakeUsers) Update(user *models.User) error {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	if r.duplicate(user) {
		return gorm.ErrDuplicatedKey
	}
	user.UpdatedAt = r.db.tick()
	r.db.users[user.ID] = *user
	return nil
}

// Delete mô phỏng DELETE thật + FK ON DELETE CASCADE.
func (r fakeUsers) Delete(id uint) error {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	delete(r.db.users, id)
	for aid, a := range r.db.articles {
		if a.AuthorID == id {
			delete(r.db.articles, aid)
			delete(r.db.articleTags, aid)
		}
	}
	for cid, c := range r.db.comments {
		if _, ok := r.db.articles[c.ArticleID]; c.UserID == id || !ok {
			delete(r.db.comments, cid)
		}
	}
	for k := range r.db.follows {
		if k[0] == id || k[1] == id {
			delete(r.db.follows, k)
		}
	}
	return nil
}

// ---------- articles ----------

type fakeArticles struct{ db *fakeDB }

// hydrate mô phỏng Preload("Author").Preload("Tags"). Gọi khi đã giữ lock.
func (r fakeArticles) hydrate(a models.Article) models.Article {
	if u, ok := r.db.users[a.AuthorID]; ok {
		a.Author = &u
	}
	a.Tags = []models.Tag{}
	for _, id := range r.db.articleTags[a.ID] {
		a.Tags = append(a.Tags, r.db.tags[id])
	}
	return a
}

func (r fakeArticles) List(f repository.ArticleFilter) ([]models.Article, int64, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()

	var matched []models.Article
	for _, a := range r.db.articles {
		a = r.hydrate(a)
		if f.Tag != "" && !slices.ContainsFunc(a.Tags, func(t models.Tag) bool { return t.Name == f.Tag }) {
			continue
		}
		if f.Author != "" && (a.Author == nil || a.Author.Username != f.Author) {
			continue
		}
		if f.FollowedBy != 0 && !r.db.follows[[2]uint{f.FollowedBy, a.AuthorID}] {
			continue
		}
		matched = append(matched, a)
	}

	sort.Slice(matched, func(i, j int) bool {
		a, b := matched[i], matched[j]
		var cmp int
		switch f.SortBy {
		case "title":
			cmp = strings.Compare(a.Title, b.Title)
		case "updated_at":
			cmp = a.UpdatedAt.Compare(b.UpdatedAt)
		default:
			cmp = a.CreatedAt.Compare(b.CreatedAt)
		}
		if cmp == 0 {
			return a.ID > b.ID
		}
		if f.Desc {
			return cmp > 0
		}
		return cmp < 0
	})

	total := int64(len(matched))
	start := min(f.Offset, len(matched))
	end := min(start+f.Limit, len(matched))
	return append([]models.Article{}, matched[start:end]...), total, nil
}

func (r fakeArticles) FindByID(id uint) (*models.Article, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	a, ok := r.db.articles[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	a = r.hydrate(a)
	return &a, nil
}

func (r fakeArticles) FindBySlug(slug string) (*models.Article, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	for _, a := range r.db.articles {
		if a.Slug == slug {
			a = r.hydrate(a)
			return &a, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (r fakeArticles) FindDetailBySlug(slug string) (*models.Article, error) {
	return r.FindBySlug(slug)
}

func (r fakeArticles) SlugsLike(base string) ([]string, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	var slugs []string
	for _, a := range r.db.articles {
		if a.Slug == base || strings.HasPrefix(a.Slug, base+"-") {
			slugs = append(slugs, a.Slug)
		}
	}
	return slugs, nil
}

func (r fakeArticles) slugTaken(slug string, exceptID uint) bool {
	for _, a := range r.db.articles {
		if a.Slug == slug && a.ID != exceptID {
			return true
		}
	}
	return false
}

func (r fakeArticles) Create(article *models.Article) error {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	if hook := r.db.beforeArticleInsert; hook != nil {
		r.db.beforeArticleInsert = nil // chỉ chạy một lần
		hook(r.db, article)
	}
	if r.slugTaken(article.Slug, 0) {
		return gorm.ErrDuplicatedKey // unique index trên slug
	}
	article.ID = r.db.nextID()
	article.CreatedAt = r.db.tick()
	article.UpdatedAt = article.CreatedAt
	for _, t := range article.Tags {
		r.db.articleTags[article.ID] = append(r.db.articleTags[article.ID], t.ID)
	}
	stored := *article
	stored.Author, stored.Tags = nil, nil
	r.db.articles[article.ID] = stored
	return nil
}

func (r fakeArticles) Update(article *models.Article, tags []models.Tag) error {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	if r.slugTaken(article.Slug, article.ID) {
		return gorm.ErrDuplicatedKey
	}
	article.UpdatedAt = r.db.tick()
	if tags != nil {
		ids := []uint{}
		for _, t := range tags {
			ids = append(ids, t.ID)
		}
		r.db.articleTags[article.ID] = ids
	}
	stored := *article
	stored.Author, stored.Tags = nil, nil
	r.db.articles[article.ID] = stored
	return nil
}

func (r fakeArticles) Delete(id uint) error {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	delete(r.db.articles, id)
	delete(r.db.articleTags, id)
	return nil
}

// ---------- tags ----------

type fakeTags struct{ db *fakeDB }

func (r fakeTags) FindAll() ([]models.Tag, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	tags := make([]models.Tag, 0, len(r.db.tags))
	for _, t := range r.db.tags {
		tags = append(tags, t)
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i].Name < tags[j].Name })
	return tags, nil
}

func (r fakeTags) FindByID(id uint) (*models.Tag, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	t, ok := r.db.tags[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return &t, nil
}

func (r fakeTags) FindByIDs(ids []uint) ([]models.Tag, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	tags := []models.Tag{}
	for _, id := range ids {
		if t, ok := r.db.tags[id]; ok {
			tags = append(tags, t)
		}
	}
	return tags, nil
}

func (r fakeTags) save(tag *models.Tag) error {
	for _, t := range r.db.tags {
		if t.ID != tag.ID && t.Name == tag.Name {
			return gorm.ErrDuplicatedKey
		}
	}
	r.db.tags[tag.ID] = *tag
	return nil
}

func (r fakeTags) Create(tag *models.Tag) error {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	tag.ID = r.db.nextID()
	tag.CreatedAt = r.db.tick()
	return r.save(tag)
}

func (r fakeTags) Update(tag *models.Tag) error {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	return r.save(tag)
}

func (r fakeTags) Delete(id uint) error {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	delete(r.db.tags, id)
	for articleID, ids := range r.db.articleTags {
		r.db.articleTags[articleID] = slices.DeleteFunc(ids, func(t uint) bool { return t == id })
	}
	return nil
}

// ---------- comments ----------

type fakeComments struct{ db *fakeDB }

func (r fakeComments) hydrate(c models.Comment) models.Comment {
	if u, ok := r.db.users[c.UserID]; ok {
		c.User = &u
	}
	return c
}

func (r fakeComments) FindByArticleID(articleID uint, limit, offset int) ([]models.Comment, int64, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	comments := []models.Comment{}
	for _, c := range r.db.comments {
		if c.ArticleID == articleID {
			comments = append(comments, r.hydrate(c))
		}
	}
	sort.Slice(comments, func(i, j int) bool { return comments[i].CreatedAt.After(comments[j].CreatedAt) })
	start := min(offset, len(comments))
	end := min(start+limit, len(comments))
	return append([]models.Comment{}, comments[start:end]...), int64(len(comments)), nil
}

func (r fakeComments) Create(comment *models.Comment) error {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	comment.ID = r.db.nextID()
	comment.CreatedAt = r.db.tick()
	comment.UpdatedAt = comment.CreatedAt
	r.db.comments[comment.ID] = *comment
	return nil
}

// ---------- follows ----------

type fakeFollows struct{ db *fakeDB }

func (r fakeFollows) Follow(followerID, followingID uint) error {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	r.db.follows[[2]uint{followerID, followingID}] = true
	return nil
}

func (r fakeFollows) Unfollow(followerID, followingID uint) error {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	delete(r.db.follows, [2]uint{followerID, followingID})
	return nil
}

func (r fakeFollows) IsFollowing(followerID, followingID uint) (bool, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	return r.db.follows[[2]uint{followerID, followingID}], nil
}
