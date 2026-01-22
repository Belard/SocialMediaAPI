package database

import "SocialMediaAPI/models"

func (d *Database) CreateUser(user *models.User) error {
	query := `INSERT INTO users (id, email, password, name, created_at) 
			  VALUES ($1, $2, $3, $4, $5)`
	_, err := d.DB.Exec(query, user.ID, user.Email, user.Password, user.Name, user.CreatedAt)
	return err
}

func (d *Database) GetUserByEmail(email string) (*models.User, error) {
	user := &models.User{}
	query := `SELECT id, email, password, name, created_at FROM users WHERE email = $1`
	err := d.DB.QueryRow(query, email).Scan(&user.ID, &user.Email, &user.Password, &user.Name, &user.CreatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (d *Database) GetUserByID(id string) (*models.User, error) {
	user := &models.User{}
	query := `SELECT id, email, password, name, created_at FROM users WHERE id = $1`
	err := d.DB.QueryRow(query, id).Scan(&user.ID, &user.Email, &user.Password, &user.Name, &user.CreatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}