package utils

// DeliveryRoutable is an interface for any entity with latitude and longitude.
type DeliveryRoutable interface {
	GetLocation() (lat float64, lng float64)
}

// OptimizeRouteNearestNeighbor sorts stops starting from origin using Nearest Neighbor heuristic.
// It returns the optimal permutation of indices [0..n-1].
func OptimizeRouteNearestNeighbor(originLat, originLng float64, points []DeliveryRoutable) []int {
	n := len(points)
	if n <= 1 {
		res := make([]int, n)
		for i := range res {
			res[i] = i
		}
		return res
	}

	visited := make([]bool, n)
	route := make([]int, 0, n)

	currentLat := originLat
	currentLng := originLng

	for step := 0; step < n; step++ {
		bestIdx := -1
		bestDist := -1.0

		for i := 0; i < n; i++ {
			if visited[i] {
				continue
			}

			pLat, pLng := points[i].GetLocation()
			dist := CalculateHaversineDistanceKm(currentLat, currentLng, pLat, pLng)

			if bestIdx == -1 || dist < bestDist {
				bestDist = dist
				bestIdx = i
			}
		}

		if bestIdx != -1 {
			visited[bestIdx] = true
			route = append(route, bestIdx)
			currentLat, currentLng = points[bestIdx].GetLocation()
		}
	}

	// Apply 2-Opt optimization pass for untangling crossing lines if n >= 4
	if n >= 4 {
		route = twoOptOptimization(originLat, originLng, points, route)
	}

	return route
}

// CalculateTotalDistance calculates the total route distance starting from origin.
func CalculateTotalDistance(originLat, originLng float64, points []DeliveryRoutable, route []int) float64 {
	if len(route) == 0 {
		return 0
	}

	total := 0.0
	currLat, currLng := originLat, originLng

	for _, idx := range route {
		pLat, pLng := points[idx].GetLocation()
		total += CalculateHaversineDistanceKm(currLat, currLng, pLat, pLng)
		currLat, currLng = pLat, pLng
	}

	return total
}

// twoOptOptimization iteratively swaps route edges to eliminate path intersections.
func twoOptOptimization(originLat, originLng float64, points []DeliveryRoutable, route []int) []int {
	n := len(route)
	bestRoute := make([]int, n)
	copy(bestRoute, route)
	bestDistance := CalculateTotalDistance(originLat, originLng, points, bestRoute)

	improved := true
	iterations := 0
	maxIterations := 50 // Limit iterations for bounded execution time

	for improved && iterations < maxIterations {
		improved = false
		iterations++

		for i := 0; i < n-1; i++ {
			for k := i + 1; k < n; k++ {
				newRoute := twoOptSwap(bestRoute, i, k)
				newDistance := CalculateTotalDistance(originLat, originLng, points, newRoute)

				if newDistance < bestDistance-0.001 { // 1 meter threshold
					bestRoute = newRoute
					bestDistance = newDistance
					improved = true
					break
				}
			}
			if improved {
				break
			}
		}
	}

	return bestRoute
}

func twoOptSwap(route []int, i, k int) []int {
	res := make([]int, len(route))
	// 1. take route[0] to route[i-1]
	for c := 0; c < i; c++ {
		res[c] = route[c]
	}
	// 2. reverse route[i] to route[k]
	dec := 0
	for c := i; c <= k; c++ {
		res[c] = route[k-dec]
		dec++
	}
	// 3. take route[k+1] to end
	for c := k + 1; c < len(route); c++ {
		res[c] = route[c]
	}
	return res
}
