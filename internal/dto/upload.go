package dto

type TempUploadResp struct {
	Key string `json:"key" example:"temp/articles/7/images/d018d0f4f7b2050d9399e96f87a97b83.png"`
	URL string `json:"url" example:"https://cdn.example.com/blog/temp/articles/7/images/d018d0f4f7b2050d9399e96f87a97b83.png"`
}
