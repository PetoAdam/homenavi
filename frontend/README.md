# Homenavi Frontend

This is the frontend for the Homenavi project, built with [React](https://react.dev/) and [Vite](https://vitejs.dev/). It provides a fast, modern, and maintainable UI for the Homenavi platform.

## Features

- ⚡️ Fast development with Vite
- 🧩 Component-based architecture using React
- 🎨 Styled with Tailwind CSS
- 🔄 Routing with React Router
- 🎉 FontAwesome icon support
- 📱 Installable as a Progressive Web App (PWA)

## Dependencies

- **react** `^19.0.0`
- **react-dom** `^19.0.0`
- **react-router-dom** `^7.5.1`
- **@fortawesome/fontawesome-svg-core** `^6.7.2`
- **@fortawesome/free-brands-svg-icons** `^6.7.2`
- **@fortawesome/free-regular-svg-icons** `^6.7.2`
- **@fortawesome/free-solid-svg-icons** `^6.7.2`
- **@fortawesome/react-fontawesome** `^0.2.2`
- **tailwindcss** `^4.1.4`
- **vite** `^6.3.1`

## Development

### Prerequisites

- [Node.js](https://nodejs.org/) (v18 or newer recommended)
- [npm](https://www.npmjs.com/) (comes with Node.js)

### Install dependencies

```bash
cd Frontend
npm install
```

### Start local development server

```bash
npm run dev
```

The app will be available at [http://localhost:5173](http://localhost:5173).

### Lint the code

```bash
npm run lint
```

### Build for production

```bash
npm run build
```

### Preview production build locally

```bash
npm run preview
```

### Enable PWA (Installable Web App)

This project uses [vite-plugin-pwa](https://vite-pwa-org.netlify.app/) to enable installation as a web app. After building or running in dev, you can "Add to Home Screen" in Chrome, Safari, etc.

To customize the app icon, name, or theme, edit `public/manifest.webmanifest` and `vite.config.js`.

## Docker Deployment

The project includes a Dockerfile and can be built and run using Docker and Docker Compose.

### Build and run with Docker Compose

From the project root:

```bash
docker-compose up --build
```

This will:

- Build the frontend using Node.js and Vite
- Serve the static files using Nginx
- Expose the app at [http://localhost:5173](http://localhost:5173)

### Manual Docker build

```bash
docker build -t homenavi-frontend .
docker run -p 5173:80 homenavi-frontend
```

## Project Structure

```
Frontend/
├── public/         # Static assets
├── src/            # React source code
├── dist/           # Production build output
├── package.json
├── tailwind.config.js
├── vite.config.js
└── ...
```

## License

This project is licensed under the MIT License.

---
